package dokploy

import (
	"context"
	"fmt"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type VolumeBackupArgs struct {
	Name            string  `pulumi:"name"`
	VolumeName      string  `pulumi:"volumeName"`
	Prefix          string  `pulumi:"prefix"`
	ApplicationID   *string `pulumi:"applicationId,optional" provider:"replaceOnChanges"`
	ComposeID       *string `pulumi:"composeId,optional" provider:"replaceOnChanges"`
	ServiceName     *string `pulumi:"serviceName,optional" provider:"replaceOnChanges"`
	DestinationID   string  `pulumi:"destinationId"`
	CronExpression  string  `pulumi:"cronExpression"`
	KeepLatestCount *int    `pulumi:"keepLatestCount,optional"`
	Enabled         bool    `pulumi:"enabled,optional"`
	TurnOff         *bool   `pulumi:"turnOff,optional"`
}

type VolumeBackupState struct {
	VolumeBackupArgs
	VolumeBackupID string `pulumi:"volumeBackupId"`
}

func (s *VolumeBackupState) Annotate(a infer.Annotator) {
	a.Describe(&s.VolumeBackupID, "The stable Dokploy volume backup ID.")
}

type VolumeBackup struct{ client clientFactory }

func (r *VolumeBackup) Annotate(a infer.Annotator) {
	a.SetToken("index", "VolumeBackup")
	a.Describe(&r, "A scheduled Dokploy volume backup for an application or Compose service. Exactly one of applicationId or composeId must be set.")
}
func (a *VolumeBackupArgs) Annotate(annotator infer.Annotator) {
	annotator.Describe(&a.Name, "The volume backup resource name.")
	annotator.Describe(&a.VolumeName, "The Docker volume name to back up.")
	annotator.Describe(&a.Prefix, "The backup file prefix.")
	annotator.Describe(&a.ApplicationID, "The optional target application ID.")
	annotator.Describe(&a.ComposeID, "The optional target Compose ID.")
	annotator.Describe(&a.ServiceName, "The Compose service name that owns the volume.")
	annotator.Describe(&a.DestinationID, "The destination ID backups are stored to.")
	annotator.Describe(&a.CronExpression, "The backup cron schedule.")
	annotator.Describe(&a.KeepLatestCount, "The optional number of most recent backups to retain.")
	annotator.Describe(&a.Enabled, "Whether the backup schedule is enabled.")
	annotator.Describe(&a.TurnOff, "Whether to turn the backup off without deleting it.")
	annotator.SetDefault(&a.Enabled, true)
}

func (a VolumeBackupArgs) validate() error {
	hasApp := a.ApplicationID != nil && *a.ApplicationID != ""
	hasCompose := a.ComposeID != nil && *a.ComposeID != ""
	hasService := a.ServiceName != nil && *a.ServiceName != ""
	if hasApp == hasCompose {
		return fmt.Errorf("exactly one of applicationId or composeId must be set")
	}
	if hasCompose && !hasService {
		return fmt.Errorf("serviceName must be set when composeId is set")
	}
	if hasApp && hasService {
		return fmt.Errorf("serviceName must not be set when applicationId is set")
	}
	return nil
}

func (r VolumeBackup) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[VolumeBackupArgs], error) {
	_, explicitEnabled := req.NewInputs.AsMap()["enabled"]
	var explicitEnabledValue bool
	if explicitEnabled {
		explicitEnabledValue = req.NewInputs.Get("enabled").AsBool()
	}
	in, failures, err := infer.DefaultCheck[VolumeBackupArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[VolumeBackupArgs]{Inputs: in, Failures: failures}, err
	}
	if !explicitEnabled {
		in.Enabled = true
	} else {
		in.Enabled = explicitEnabledValue
	}
	for _, field := range []struct {
		name, value string
	}{{"name", in.Name}, {"volumeName", in.VolumeName}, {"prefix", in.Prefix}, {"destinationId", in.DestinationID}, {"cronExpression", in.CronExpression}} {
		if field.value == "" && !req.NewInputs.Get(field.name).HasComputed() {
			failures = append(failures, p.CheckFailure{Property: field.name, Reason: fmt.Sprintf("%s must not be empty", field.name)})
		}
	}
	targetComputed := req.NewInputs.Get("applicationId").HasComputed() || req.NewInputs.Get("composeId").HasComputed()
	if !targetComputed {
		if err := in.validate(); err != nil {
			failures = append(failures, p.CheckFailure{Property: "target", Reason: err.Error()})
		}
	}
	return infer.CheckResponse[VolumeBackupArgs]{Inputs: in, Failures: failures}, nil
}

func (r VolumeBackup) Diff(_ context.Context, req infer.DiffRequest[VolumeBackupArgs, VolumeBackupState]) (infer.DiffResponse, error) {
	in, old := req.Inputs, req.State.VolumeBackupArgs
	d := map[string]p.PropertyDiff{}
	for _, field := range []struct {
		name    string
		changed bool
	}{
		{"applicationId", !sameOptionalString(in.ApplicationID, old.ApplicationID)}, {"composeId", !sameOptionalString(in.ComposeID, old.ComposeID)},
		{"serviceName", !sameOptionalString(in.ServiceName, old.ServiceName)},
	} {
		if field.changed {
			d[field.name] = p.PropertyDiff{Kind: p.UpdateReplace}
		}
	}
	for _, field := range []struct {
		name    string
		changed bool
	}{
		{"name", in.Name != old.Name}, {"volumeName", in.VolumeName != old.VolumeName}, {"prefix", in.Prefix != old.Prefix},
		{"destinationId", in.DestinationID != old.DestinationID}, {"cronExpression", in.CronExpression != old.CronExpression},
		{"keepLatestCount", !sameOptionalInt(in.KeepLatestCount, old.KeepLatestCount)}, {"enabled", in.Enabled != old.Enabled},
		{"turnOff", !sameOptionalBool(in.TurnOff, old.TurnOff)},
	} {
		if field.changed {
			d[field.name] = p.PropertyDiff{Kind: p.Update}
		}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func volumeBackupBody(a VolumeBackupArgs) (applicationID, composeID, serviceName nullable.Nullable[string], keepLatestCount nullable.Nullable[float32], serviceType string) {
	applicationID, composeID, serviceName = nullable.NewNullNullable[string](), nullable.NewNullNullable[string](), nullable.NewNullNullable[string]()
	keepLatestCount = nullable.NewNullNullable[float32]()
	if a.KeepLatestCount != nil {
		keepLatestCount = nullable.NewNullableWithValue(float32(*a.KeepLatestCount))
	}
	switch {
	case a.ApplicationID != nil:
		applicationID = nullable.NewNullableWithValue(*a.ApplicationID)
		serviceType = "application"
	case a.ComposeID != nil:
		composeID = nullable.NewNullableWithValue(*a.ComposeID)
		serviceType = "compose"
		if a.ServiceName != nil {
			serviceName = nullable.NewNullableWithValue(*a.ServiceName)
		}
	}
	return applicationID, composeID, serviceName, keepLatestCount, serviceType
}

func (r VolumeBackup) Create(ctx context.Context, req infer.CreateRequest[VolumeBackupArgs]) (infer.CreateResponse[VolumeBackupState], error) {
	state := VolumeBackupState{VolumeBackupArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[VolumeBackupState]{Output: state}, nil
	}
	applicationID, composeID, serviceName, keepLatestCount, serviceType := volumeBackupBody(req.Inputs)
	b := generated.VolumeBackupsCreateJSONRequestBody{
		Name: req.Inputs.Name, VolumeName: req.Inputs.VolumeName, Prefix: req.Inputs.Prefix, DestinationId: req.Inputs.DestinationID,
		CronExpression: req.Inputs.CronExpression, Enabled: nullable.NewNullableWithValue(req.Inputs.Enabled),
		ApplicationId: applicationID, ComposeId: composeID, ServiceName: serviceName, KeepLatestCount: keepLatestCount,
		ServiceType: ptr(generated.VolumeBackupsCreateJSONBodyServiceType(serviceType)),
	}
	if req.Inputs.TurnOff != nil {
		b.TurnOff = req.Inputs.TurnOff
	}
	resp, err := r.client(ctx).VolumeBackupsCreateWithResponse(ctx, b)
	if err != nil {
		return infer.CreateResponse[VolumeBackupState]{}, err
	}
	if resp.JSON200 == nil || resp.JSON200.VolumeBackupId == nil {
		return infer.CreateResponse[VolumeBackupState]{}, fmt.Errorf("volumeBackups.create returned incomplete volume backup")
	}
	state.VolumeBackupID = *resp.JSON200.VolumeBackupId
	return infer.CreateResponse[VolumeBackupState]{ID: state.VolumeBackupID, Output: state}, nil
}

func (r VolumeBackup) Read(ctx context.Context, req infer.ReadRequest[VolumeBackupArgs, VolumeBackupState]) (infer.ReadResponse[VolumeBackupArgs, VolumeBackupState], error) {
	resp, err := r.client(ctx).VolumeBackupsOneWithResponse(ctx, &generated.VolumeBackupsOneParams{VolumeBackupId: req.ID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[VolumeBackupArgs, VolumeBackupState]{ID: ""}, nil
		}
		return infer.ReadResponse[VolumeBackupArgs, VolumeBackupState]{}, err
	}
	if resp.JSON200 == nil || resp.JSON200.VolumeBackupId == nil {
		return infer.ReadResponse[VolumeBackupArgs, VolumeBackupState]{}, fmt.Errorf("volumeBackups.one returned incomplete volume backup")
	}
	v := resp.JSON200
	a := req.State.VolumeBackupArgs
	a.Name, a.VolumeName, a.Prefix = value(v.Name), value(v.VolumeName), value(v.Prefix)
	a.DestinationID, a.CronExpression = value(v.DestinationId), value(v.CronExpression)
	a.KeepLatestCount, a.TurnOff = v.KeepLatestCount, v.TurnOff
	a.Enabled = req.State.Enabled
	if v.Enabled != nil {
		a.Enabled = *v.Enabled
	}
	a.ApplicationID, a.ComposeID, a.ServiceName = nil, nil, nil
	switch value(v.ServiceType) {
	case "application":
		a.ApplicationID = v.ApplicationId
	case "compose":
		a.ComposeID, a.ServiceName = v.ComposeId, v.ServiceName
	}
	state := VolumeBackupState{VolumeBackupArgs: a, VolumeBackupID: *v.VolumeBackupId}
	return infer.ReadResponse[VolumeBackupArgs, VolumeBackupState]{ID: *v.VolumeBackupId, Inputs: a, State: state}, nil
}

func (r VolumeBackup) Update(ctx context.Context, req infer.UpdateRequest[VolumeBackupArgs, VolumeBackupState]) (infer.UpdateResponse[VolumeBackupState], error) {
	state := VolumeBackupState{VolumeBackupArgs: req.Inputs, VolumeBackupID: req.ID}
	if req.DryRun {
		return infer.UpdateResponse[VolumeBackupState]{Output: state}, nil
	}
	applicationID, composeID, serviceName, keepLatestCount, serviceType := volumeBackupBody(req.Inputs)
	b := generated.VolumeBackupsUpdateJSONRequestBody{
		VolumeBackupId: req.ID, Name: req.Inputs.Name, VolumeName: req.Inputs.VolumeName, Prefix: req.Inputs.Prefix,
		DestinationId: req.Inputs.DestinationID, CronExpression: req.Inputs.CronExpression, Enabled: nullable.NewNullableWithValue(req.Inputs.Enabled),
		ApplicationId: applicationID, ComposeId: composeID, ServiceName: serviceName, KeepLatestCount: keepLatestCount,
		ServiceType: ptr(generated.VolumeBackupsUpdateJSONBodyServiceType(serviceType)),
	}
	if req.Inputs.TurnOff != nil {
		b.TurnOff = req.Inputs.TurnOff
	}
	if _, err := r.client(ctx).VolumeBackupsUpdateWithResponse(ctx, b); err != nil {
		return infer.UpdateResponse[VolumeBackupState]{Output: state}, err
	}
	return infer.UpdateResponse[VolumeBackupState]{Output: state}, nil
}

func (r VolumeBackup) Delete(ctx context.Context, req infer.DeleteRequest[VolumeBackupState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).VolumeBackupsDeleteWithResponse(ctx, generated.VolumeBackupsDeleteJSONRequestBody{VolumeBackupId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, err
}

func (r VolumeBackup) WireDependencies(f infer.FieldSelector, args *VolumeBackupArgs, state *VolumeBackupState) {
	deps := []infer.InputField{
		f.InputField(&args.Name), f.InputField(&args.VolumeName), f.InputField(&args.Prefix), f.InputField(&args.ApplicationID),
		f.InputField(&args.ComposeID), f.InputField(&args.ServiceName), f.InputField(&args.DestinationID), f.InputField(&args.CronExpression),
		f.InputField(&args.KeepLatestCount), f.InputField(&args.Enabled), f.InputField(&args.TurnOff),
	}
	f.OutputField(&state.VolumeBackupID).DependsOn(deps...)
}
