package dokploy

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/stretchr/testify/require"
)

type liveScheduleFixtureOps struct {
	create    func(context.Context, ScheduleArgs) (infer.CreateResponse[ScheduleState], error)
	read      func(context.Context, string) (infer.ReadResponse[ScheduleArgs, ScheduleState], error)
	remove    func(context.Context, string) error
	register  func(string, func()) func()
	exercise  func(context.Context, string, infer.ReadResponse[ScheduleArgs, ScheduleState]) error
	uncertain func()
}

// Retain only targets with unresolved Schedule ownership/absence. A project
// containing such a target must also survive, since its deletion can cascade.
type liveScheduleCleanupGuard struct{ pending map[string]int }

func newLiveScheduleCleanupGuard() *liveScheduleCleanupGuard {
	return &liveScheduleCleanupGuard{pending: make(map[string]int)}
}

func (g *liveScheduleCleanupGuard) hold(typ string) func() {
	g.pending[typ]++
	resolved := false
	return func() {
		if !resolved {
			g.pending[typ]--
			resolved = true
		}
	}
}

func (g *liveScheduleCleanupGuard) cleanupTarget(typ string, cleanup func()) bool {
	if g.pending[typ] != 0 {
		return false
	}
	cleanup()
	return true
}

func (g *liveScheduleCleanupGuard) retainProjectIfNeeded(release func()) {
	if g.pending["application"] != 0 || g.pending["compose"] != 0 {
		release()
	}
}

// Ownership is registered before inspecting the create error: the provider
// preserves an acknowledged-create ID even when read-back returns partial state.
// An unacknowledged create is never retried, discovered by name, or deleted.
func runScheduleLiveFixture(t *testing.T, ctx context.Context, args ScheduleArgs, prerequisite bool, ops liveScheduleFixtureOps, guard *liveScheduleCleanupGuard) error {
	t.Helper()
	if !prerequisite {
		t.Skip("Schedule coverage requires a disposable target or an explicit dedicated server scope and opt-in")
	}
	args.Enabled = false
	absent := guard.hold(args.ScheduleType)
	created, err := ops.create(ctx, args)
	if created.ID == "" {
		ops.uncertain()
		return errors.New("Schedule create ownership is uncertain; stop and investigate before retrying")
	}
	release := ops.register(created.ID, absent)
	if err != nil {
		return errors.New("Schedule acknowledged create could not be confirmed")
	}
	read, err := ops.read(ctx, created.ID)
	if err != nil || read.ID != created.ID || read.Inputs.Enabled {
		return errors.New("Schedule disabled read-back could not be confirmed")
	}
	if ops.exercise != nil {
		if err := ops.exercise(ctx, created.ID, read); err != nil {
			return errors.New("Schedule disabled lifecycle failed")
		}
	}
	cleanupCtx, cancel := cleanupContext()
	defer cancel()
	err = deleteAndVerifyLiveOwned(cleanupCtx, func() error {
		return ops.remove(cleanupCtx, created.ID)
	}, func() (string, error) {
		gone, readErr := ops.read(cleanupCtx, created.ID)
		return gone.ID, readErr
	}, func() {
		absent()
		release()
	})
	if err != nil {
		return errors.New("Schedule deletion or absence verification failed")
	}
	return nil
}

func scheduleLivePrerequisite(typ, targetID, optIn, serverScope string) bool {
	switch typ {
	case "application", "compose":
		return targetID != ""
	case "server", "dokploy-server":
		return optIn == "1" && serverScope != ""
	default:
		return false
	}
}

func runLiveScheduleCases(t *testing.T, ctx context.Context, api *client.Client, applicationID, composeID string, guard *liveScheduleCleanupGuard) {
	t.Helper()
	serverScope := os.Getenv("DOKPLOY_ACCEPTANCE_SERVER_ID")
	optIn := os.Getenv("DOKPLOY_ACCEPTANCE_ALLOW_SERVER_SCHEDULES")
	for _, target := range []struct{ typ, id string }{
		{"application", applicationID}, {"compose", composeID}, {"server", serverScope}, {"dokploy-server", ""},
	} {
		t.Run(target.typ, func(t *testing.T) {
			if heavyLiveTierStopped() {
				t.Skip("live acceptance stopped after a cleanup or server-health failure")
			}
			args := ScheduleArgs{Name: liveRunName("schedule"), CronExpression: "0 0 1 1 *", Command: "true", ScheduleType: target.typ, Enabled: false}
			switch target.typ {
			case "application":
				args.ApplicationID = &target.id
			case "compose":
				args.ComposeID, args.ServiceName = &target.id, stringPtr("web")
			case "server":
				args.ServerID = &target.id
			}
			r := Schedule{client: fixedClient(api)}
			ops := liveScheduleFixtureOps{
				create: func(c context.Context, a ScheduleArgs) (infer.CreateResponse[ScheduleState], error) {
					return r.Create(c, infer.CreateRequest[ScheduleArgs]{Inputs: a})
				},
				read: func(c context.Context, id string) (infer.ReadResponse[ScheduleArgs, ScheduleState], error) {
					return r.Read(c, infer.ReadRequest[ScheduleArgs, ScheduleState]{ID: id})
				},
				remove: func(c context.Context, id string) error {
					_, err := r.Delete(c, infer.DeleteRequest[ScheduleState]{ID: id})
					return err
				},
				uncertain: func() {
					// No acknowledged identity exists for safe fallback cleanup.
					// Stop rather than probing or retrying this uncertainty live.
					recordStructuralCleanup("schedule", errors.New("create ownership uncertain"))
				},
				exercise: func(c context.Context, id string, read infer.ReadResponse[ScheduleArgs, ScheduleState]) error {
					updated := read.Inputs
					updated.Name += "-updated"
					updated.Description = stringPtr("disabled acceptance schedule")
					updated.Enabled = false
					_, err := r.Update(c, infer.UpdateRequest[ScheduleArgs, ScheduleState]{ID: id, Inputs: updated, State: read.State})
					if err != nil {
						return err
					}
					imported, err := r.Read(c, infer.ReadRequest[ScheduleArgs, ScheduleState]{ID: id})
					if err != nil {
						return err
					}
					if imported.ID != id || imported.Inputs.Enabled || imported.Inputs.Name != updated.Name || !sameOptionalString(imported.Inputs.Description, updated.Description) {
						return errors.New("disabled Schedule update/import mismatch")
					}
					return nil
				},
			}
			ops.register = func(id string, absent func()) func() {
				return registerLiveCleanup(t, "schedule", id, func(c context.Context) error {
					return ops.remove(c, id)
				}, func(c context.Context) (string, error) {
					gone, err := ops.read(c, id)
					if err == nil && gone.ID == "" {
						absent()
					}
					return gone.ID, err
				})
			}
			err := runScheduleLiveFixture(t, ctx, args, scheduleLivePrerequisite(target.typ, target.id, optIn, serverScope), ops, guard)
			requireLiveLifecycleNoError(t, "Schedule", "disabled lifecycle", err)
		})
		if heavyLiveTierStopped() {
			t.Fatal("Schedule cleanup or ownership uncertainty requires operator investigation before continuing")
		}
	}
}

func TestScheduleLiveFixtureDisabledAndCleanupOrdered(t *testing.T) {
	for _, phase := range []string{"success", "partial-create", "read-error", "read-enabled", "read-wrong-id", "exercise-error", "delete-error", "uncertain-create"} {
		t.Run(phase, func(t *testing.T) {
			var events []string
			present := false
			acknowledged := phase != "uncertain-create"
			guard := newLiveScheduleCleanupGuard()
			ops := liveScheduleFixtureOps{
				create: func(_ context.Context, a ScheduleArgs) (infer.CreateResponse[ScheduleState], error) {
					require.False(t, a.Enabled, "fixture must never enable command execution")
					events = append(events, "create")
					if !acknowledged {
						return infer.CreateResponse[ScheduleState]{}, errors.New("untrusted response details")
					}
					present = true
					created := infer.CreateResponse[ScheduleState]{ID: "offline-schedule", Output: ScheduleState{ScheduleArgs: a, ScheduleID: "offline-schedule"}}
					if phase == "partial-create" {
						return created, errors.New("untrusted response details")
					}
					return created, nil
				},
				read: func(_ context.Context, id string) (infer.ReadResponse[ScheduleArgs, ScheduleState], error) {
					require.Equal(t, "offline-schedule", id)
					if !present {
						events = append(events, "absence")
						return infer.ReadResponse[ScheduleArgs, ScheduleState]{}, nil
					}
					events = append(events, "read")
					if phase == "read-error" && len(events) == 3 {
						return infer.ReadResponse[ScheduleArgs, ScheduleState]{}, errors.New("untrusted response details")
					}
					if phase == "read-wrong-id" {
						return infer.ReadResponse[ScheduleArgs, ScheduleState]{ID: "unrelated-offline-schedule"}, nil
					}
					return infer.ReadResponse[ScheduleArgs, ScheduleState]{ID: id, Inputs: ScheduleArgs{Enabled: phase == "read-enabled"}}, nil
				},
				remove: func(_ context.Context, id string) error {
					require.Equal(t, "offline-schedule", id)
					events = append(events, "delete")
					if phase == "delete-error" && len(events) == 4 {
						return errors.New("untrusted response details")
					}
					present = false
					return nil
				},
				uncertain: func() { events = append(events, "stop") },
			}
			if phase == "exercise-error" {
				ops.exercise = func(context.Context, string, infer.ReadResponse[ScheduleArgs, ScheduleState]) error {
					events = append(events, "exercise")
					return errors.New("untrusted response details")
				}
			}
			t.Run("fixture", func(t *testing.T) {
				t.Cleanup(func() { guard.cleanupTarget("application", func() { events = append(events, "target-cleanup") }) })
				ops.register = func(id string, absent func()) func() {
					events = append(events, "register")
					return registerLiveCleanup(t, "schedule", id, func(c context.Context) error {
						return ops.remove(c, id)
					}, func(c context.Context) (string, error) {
						v, err := ops.read(c, id)
						if err == nil && v.ID == "" {
							absent()
						}
						return v.ID, err
					})
				}
				err := runScheduleLiveFixture(t, context.Background(), ScheduleArgs{ScheduleType: "application", Enabled: true}, true, ops, guard)
				if phase == "success" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
					require.NotContains(t, err.Error(), "untrusted response details")
				}
			})
			want := map[string][]string{
				"success":          {"create", "register", "read", "delete", "absence", "target-cleanup"},
				"partial-create":   {"create", "register", "delete", "absence", "target-cleanup"},
				"read-error":       {"create", "register", "read", "delete", "absence", "target-cleanup"},
				"read-enabled":     {"create", "register", "read", "delete", "absence", "target-cleanup"},
				"read-wrong-id":    {"create", "register", "read", "delete", "absence", "target-cleanup"},
				"exercise-error":   {"create", "register", "read", "exercise", "delete", "absence", "target-cleanup"},
				"delete-error":     {"create", "register", "read", "delete", "delete", "absence", "target-cleanup"},
				"uncertain-create": {"create", "stop"},
			}
			require.Equal(t, want[phase], events)
		})
	}
	var skipped bool
	t.Run("missing-prerequisite", func(t *testing.T) {
		t.Cleanup(func() { skipped = t.Skipped() })
		err := runScheduleLiveFixture(t, context.Background(), ScheduleArgs{}, false, liveScheduleFixtureOps{}, newLiveScheduleCleanupGuard())
		t.Fatalf("missing prerequisite did not skip: %v", err)
	})
	require.True(t, skipped)
}

func TestScheduleLiveServerPrerequisites(t *testing.T) {
	for _, typ := range []string{"server", "dokploy-server"} {
		for _, tc := range []struct {
			optIn, scope string
			want         bool
		}{{"", "", false}, {"1", "", false}, {"", "offline-scope", false}, {"true", "offline-scope", false}, {"1", "offline-scope", true}} {
			require.Equal(t, tc.want, scheduleLivePrerequisite(typ, "", tc.optIn, tc.scope))
		}
	}
	require.False(t, scheduleLivePrerequisite("application", "", "1", "offline-scope"))
	require.True(t, scheduleLivePrerequisite("application", "offline-target", "", ""))
	require.True(t, scheduleLivePrerequisite("compose", "offline-target", "", ""))
}

func TestScheduleLiveParentCleanupRetainsUnresolvedTargets(t *testing.T) {
	for _, phase := range []string{"persistent-delete-error", "uncertain-create"} {
		for _, typ := range []string{"application", "compose"} {
			t.Run(phase+"/"+typ, func(t *testing.T) {
				guard := newLiveScheduleCleanupGuard()
				var targetDeleted, unrelatedDeleted, projectDeleted bool
				var stopped bool
				deleteAttempts := 0
				t.Run("parent", func(t *testing.T) {
					projectOwner := newLiveCleanupOwner(func() { projectDeleted = true })
					t.Cleanup(projectOwner.cleanupOnce)
					t.Cleanup(func() { guard.retainProjectIfNeeded(projectOwner.release) })
					t.Cleanup(func() {
						guard.cleanupTarget(typ, func() { targetDeleted = true })
						other := "compose"
						if typ == "compose" {
							other = "application"
						}
						guard.cleanupTarget(other, func() { unrelatedDeleted = true })
					})
					t.Run("schedule", func(t *testing.T) {
						ops := liveScheduleFixtureOps{
							create: func(context.Context, ScheduleArgs) (infer.CreateResponse[ScheduleState], error) {
								if phase == "uncertain-create" {
									return infer.CreateResponse[ScheduleState]{}, errors.New("private response details")
								}
								return infer.CreateResponse[ScheduleState]{ID: "offline-schedule"}, nil
							},
							read: func(context.Context, string) (infer.ReadResponse[ScheduleArgs, ScheduleState], error) {
								return infer.ReadResponse[ScheduleArgs, ScheduleState]{ID: "offline-schedule"}, nil
							},
							remove: func(context.Context, string) error {
								deleteAttempts++
								return errors.New("private response details")
							},
							uncertain: func() { stopped = true },
						}
						ops.register = func(id string, absent func()) func() {
							owner := newLiveCleanupOwner(func() {
								err := verifyLiveCleanup(context.Background(), func(c context.Context) error { return ops.remove(c, id) }, func(c context.Context) (string, error) {
									read, err := ops.read(c, id)
									return read.ID, err
								})
								if err != nil {
									stopped = true
									return
								}
								absent()
							})
							t.Cleanup(owner.cleanupOnce)
							return owner.release
						}
						err := runScheduleLiveFixture(t, context.Background(), ScheduleArgs{ScheduleType: typ}, true, ops, guard)
						require.Error(t, err)
						require.NotContains(t, err.Error(), "private response details")
					})
				})
				require.False(t, targetDeleted, "unresolved schedule must retain its target")
				require.False(t, projectDeleted, "project deletion must not cascade into retained targets")
				require.True(t, unrelatedDeleted, "unrelated workload cleanup remains safe")
				require.True(t, stopped)
				if phase == "persistent-delete-error" {
					require.Equal(t, 2, deleteAttempts, "explicit and fallback deletion must both be attempted")
				} else {
					require.Zero(t, deleteAttempts, "uncertain identity must not be deleted")
				}
			})
		}
	}
}
