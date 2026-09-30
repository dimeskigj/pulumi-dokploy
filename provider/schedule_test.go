package dokploy

import (
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestScheduleCheckTypesAndTargets(t *testing.T) {
	for _, tc := range []struct {
		typ                  string
		app, compose, server bool
		valid                bool
	}{{"application", true, false, false, true}, {"compose", false, true, false, true}, {"server", false, false, true, true}, {"dokploy-server", false, false, false, true}, {"other", false, false, false, false}} {
		a := ScheduleArgs{Name: "n", CronExpression: "* * * * *", Command: "echo", ScheduleType: tc.typ}
		if tc.app {
			a.ApplicationID = ptr("a")
		}
		if tc.compose {
			a.ComposeID = ptr("c")
		}
		if tc.server {
			a.ServerID = ptr("s")
		}
		values := map[string]property.Value{"name": property.New(a.Name), "cronExpression": property.New(a.CronExpression), "command": property.New(a.Command), "scheduleType": property.New(a.ScheduleType)}
		if a.ApplicationID != nil {
			values["applicationId"] = property.New(*a.ApplicationID)
		}
		if a.ComposeID != nil {
			values["composeId"] = property.New(*a.ComposeID)
		}
		if a.ServerID != nil {
			values["serverId"] = property.New(*a.ServerID)
		}
		pm := property.NewMap(values)
		got, err := (Schedule{}).Check(t.Context(), infer.CheckRequest{NewInputs: pm})
		require.NoError(t, err)
		require.Equal(t, tc.valid, len(got.Failures) == 0, tc.typ)
	}
}

func TestScheduleCheckDefersComputedTargets(t *testing.T) {
	pm := property.NewMap(map[string]property.Value{"name": property.New("n"), "cronExpression": property.New("* * * * *"), "command": property.New("echo"), "scheduleType": property.New("application"), "applicationId": property.New(property.Computed)})
	got, err := (Schedule{}).Check(t.Context(), infer.CheckRequest{NewInputs: pm})
	require.NoError(t, err)
	require.Empty(t, got.Failures)
}

func TestScheduleCheckRejectsMissingRequiredFieldsAndInvalidShell(t *testing.T) {
	inputs := property.NewMap(map[string]property.Value{"name": property.New(""), "cronExpression": property.New(""), "command": property.New(""), "scheduleType": property.New("dokploy-server"), "shellType": property.New("zsh")})
	got, err := (Schedule{}).Check(t.Context(), infer.CheckRequest{NewInputs: inputs})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"name", "cronExpression", "command", "shellType"}, failureProperties(got.Failures))
}

func TestScheduleCheckRejectsIncompatibleTargets(t *testing.T) {
	inputs := property.NewMap(map[string]property.Value{"name": property.New("n"), "cronExpression": property.New("* * * * *"), "command": property.New("echo"), "scheduleType": property.New("application"), "applicationId": property.New("a"), "serverId": property.New("s")})
	got, err := (Schedule{}).Check(t.Context(), infer.CheckRequest{NewInputs: inputs})
	require.NoError(t, err)
	require.Equal(t, []string{"serverId"}, failureProperties(got.Failures))
}

func TestScheduleCheckRequiresTypeSpecificTarget(t *testing.T) {
	inputs := property.NewMap(map[string]property.Value{"name": property.New("n"), "cronExpression": property.New("* * * * *"), "command": property.New("echo"), "scheduleType": property.New("server")})
	got, err := (Schedule{}).Check(t.Context(), infer.CheckRequest{NewInputs: inputs})
	require.NoError(t, err)
	require.Equal(t, []string{"serverId"}, failureProperties(got.Failures))
}

func TestScheduleDiffReplacementAndUpdates(t *testing.T) {
	old := ScheduleArgs{Name: "n", CronExpression: "* * * * *", Command: "echo", ScheduleType: "application", ApplicationID: ptr("a"), AppName: ptr("app")}
	next := old
	next.ApplicationID = ptr("b")
	next.Name = "new"
	d, err := (Schedule{}).Diff(t.Context(), infer.DiffRequest[ScheduleArgs, ScheduleState]{Inputs: next, State: ScheduleState{ScheduleArgs: old}})
	require.NoError(t, err)
	require.Equal(t, p.UpdateReplace, d.DetailedDiff["applicationId"].Kind)
	require.True(t, d.DeleteBeforeReplace)
	require.Equal(t, p.Update, d.DetailedDiff["name"].Kind)
	equal, err := (Schedule{}).Diff(t.Context(), infer.DiffRequest[ScheduleArgs, ScheduleState]{Inputs: old, State: ScheduleState{ScheduleArgs: old}})
	require.NoError(t, err)
	require.False(t, equal.HasChanges)
}
