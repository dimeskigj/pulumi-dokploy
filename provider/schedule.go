package dokploy

import (
	"context"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type ScheduleArgs struct {
	Name           string  `pulumi:"name"`
	CronExpression string  `pulumi:"cronExpression"`
	Command        string  `pulumi:"command" provider:"secret"`
	ScheduleType   string  `pulumi:"scheduleType" provider:"replaceOnChanges"`
	Description    *string `pulumi:"description,optional"`
	AppName        *string `pulumi:"appName,optional" provider:"replaceOnChanges"`
	ServiceName    *string `pulumi:"serviceName,optional" provider:"replaceOnChanges"`
	ShellType      *string `pulumi:"shellType,optional"`
	Script         *string `pulumi:"script,optional" provider:"secret"`
	Timezone       *string `pulumi:"timezone,optional"`
	OrganizationID *string `pulumi:"organizationId,optional"`
	ApplicationID  *string `pulumi:"applicationId,optional" provider:"replaceOnChanges"`
	ComposeID      *string `pulumi:"composeId,optional" provider:"replaceOnChanges"`
	ServerID       *string `pulumi:"serverId,optional" provider:"replaceOnChanges"`
	Enabled        bool    `pulumi:"enabled,optional" provider:"default:false"`
}
type ScheduleState struct {
	ScheduleArgs
	ScheduleID string `pulumi:"scheduleId"`
}
type Schedule struct{ client clientFactory }

func (a *ScheduleArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.Name, "Schedule name.")
	n.Describe(&a.CronExpression, "Cron expression for the schedule.")
	n.Describe(&a.Command, "Command executed by the schedule.")
	n.Describe(&a.ScheduleType, "Schedule target type: application, compose, server, or dokploy-server.")
	n.Describe(&a.Description, "Optional schedule description.")
	n.Describe(&a.AppName, "Optional application name.")
	n.Describe(&a.ServiceName, "Optional Compose service name.")
	n.Describe(&a.ShellType, "Shell used to execute the command (bash or sh).")
	n.Describe(&a.Script, "Optional script executed by the schedule.")
	n.Describe(&a.Timezone, "Timezone for the schedule.")
	n.Describe(&a.OrganizationID, "Dokploy organization ID.")
	n.Describe(&a.ApplicationID, "Target application ID.")
	n.Describe(&a.ComposeID, "Target Compose ID.")
	n.Describe(&a.ServerID, "Target server ID.")
	n.Describe(&a.Enabled, "Whether the schedule is enabled; defaults to false.")
	n.SetDefault(&a.Enabled, false)
}
func (a *ScheduleState) Annotate(n infer.Annotator) {
	n.Describe(&a.ScheduleID, "Stable Dokploy schedule ID.")
}
func (r *Schedule) Annotate(n infer.Annotator) {
	n.SetToken("index", "Schedule")
	n.Describe(r, "A Dokploy scheduled command.")
}

func (r Schedule) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[ScheduleArgs], error) {
	in, failures, err := infer.DefaultCheck[ScheduleArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[ScheduleArgs]{Inputs: in, Failures: failures}, err
	}
	add := func(field, reason string) {
		failures = append(failures, p.CheckFailure{Property: field, Reason: reason})
	}
	for _, f := range []struct{ name, value string }{{"name", in.Name}, {"cronExpression", in.CronExpression}, {"command", in.Command}} {
		if f.value == "" && !req.NewInputs.Get(f.name).HasComputed() {
			add(f.name, f.name+" must not be empty")
		}
	}
	targets := map[string]*string{"applicationId": in.ApplicationID, "composeId": in.ComposeID, "serverId": in.ServerID}
	computed := func(k string) bool { return req.NewInputs.Get(k).HasComputed() }
	known := in.ScheduleType != "" && !req.NewInputs.Get("scheduleType").HasComputed()
	valid := map[string]string{"application": "applicationId", "compose": "composeId", "server": "serverId", "dokploy-server": ""}
	if known {
		required, ok := valid[in.ScheduleType]
		if !ok {
			add("scheduleType", "scheduleType must be application, compose, server, or dokploy-server")
		} else if required != "" && targets[required] == nil && !computed(required) {
			add(required, required+" is required for scheduleType "+in.ScheduleType)
		}
		for k, v := range targets {
			if k != required && v != nil && !computed(k) {
				add(k, k+" is incompatible with scheduleType "+in.ScheduleType)
			}
		}
	}
	if in.ShellType != nil && !req.NewInputs.Get("shellType").HasComputed() && *in.ShellType != "bash" && *in.ShellType != "sh" {
		add("shellType", "shellType must be bash or sh")
	}
	return infer.CheckResponse[ScheduleArgs]{Inputs: in, Failures: failures}, nil
}
func (r Schedule) Diff(_ context.Context, req infer.DiffRequest[ScheduleArgs, ScheduleState]) (infer.DiffResponse, error) {
	a, b := req.Inputs, req.State.ScheduleArgs
	d := map[string]p.PropertyDiff{}
	fields := []struct {
		name             string
		changed, replace bool
	}{{"name", a.Name != b.Name, false}, {"cronExpression", a.CronExpression != b.CronExpression, false}, {"command", a.Command != b.Command, false}, {"scheduleType", a.ScheduleType != b.ScheduleType, true}, {"description", !sameOptionalString(a.Description, b.Description), false}, {"appName", !sameOptionalString(a.AppName, b.AppName), true}, {"serviceName", !sameOptionalString(a.ServiceName, b.ServiceName), true}, {"shellType", !sameOptionalString(a.ShellType, b.ShellType), false}, {"script", !sameOptionalString(a.Script, b.Script), false}, {"timezone", !sameOptionalString(a.Timezone, b.Timezone), false}, {"organizationId", !sameOptionalString(a.OrganizationID, b.OrganizationID), false}, {"applicationId", !sameOptionalString(a.ApplicationID, b.ApplicationID), true}, {"composeId", !sameOptionalString(a.ComposeID, b.ComposeID), true}, {"serverId", !sameOptionalString(a.ServerID, b.ServerID), true}, {"enabled", a.Enabled != b.Enabled, false}}
	for _, f := range fields {
		if f.changed {
			kind := p.Update
			if f.replace {
				kind = p.UpdateReplace
			}
			d[f.name] = p.PropertyDiff{Kind: kind}
		}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d, DeleteBeforeReplace: hasReplacement(d)}, nil
}
