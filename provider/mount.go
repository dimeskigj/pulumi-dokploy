package dokploy

import (
	"context"
	"errors"
	"fmt"
	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
	"strings"
)

type MountArgs struct {
	Type          string  `pulumi:"type" provider:"replaceOnChanges"`
	MountPath     string  `pulumi:"mountPath"`
	HostPath      *string `pulumi:"hostPath,optional"`
	VolumeName    *string `pulumi:"volumeName,optional"`
	FilePath      *string `pulumi:"filePath,optional"`
	Content       *string `pulumi:"content,optional" provider:"secret"`
	ApplicationID *string `pulumi:"applicationId,optional" provider:"replaceOnChanges"`
	ComposeID     *string `pulumi:"composeId,optional" provider:"replaceOnChanges"`
	PostgresID    *string `pulumi:"postgresId,optional" provider:"replaceOnChanges"`
	MySQLID       *string `pulumi:"mysqlId,optional" provider:"replaceOnChanges"`
	MariaDBID     *string `pulumi:"mariadbId,optional" provider:"replaceOnChanges"`
	RedisID       *string `pulumi:"redisId,optional" provider:"replaceOnChanges"`
}

const (
	mountTypeBind   = "bind"
	mountTypeVolume = "volume"
	mountTypeFile   = "file"
)

type MountState struct {
	MountArgs
	MountID string `pulumi:"mountId"`
}
type Mount struct{ client clientFactory }

func (r *Mount) Annotate(a infer.Annotator) {
	a.SetToken("index", "Mount")
	a.Describe(&r, "A Dokploy workload mount.")
}
func (s *MountState) Annotate(a infer.Annotator) {
	a.Describe(&s.MountID, "The stable Dokploy mount ID.")
}
func (a *MountArgs) Annotate(n infer.Annotator) {
	for _, v := range []struct {
		p any
		d string
	}{{&a.Type, "The mount type."}, {&a.MountPath, "The path inside the workload."}, {&a.HostPath, "The host path."}, {&a.VolumeName, "The volume name."}, {&a.FilePath, "The file path."}, {&a.Content, "The file content."}, {&a.ApplicationID, "The application target."}, {&a.ComposeID, "The Compose target."}, {&a.PostgresID, "The Postgres target."}, {&a.MySQLID, "The MySQL target."}, {&a.MariaDBID, "The MariaDB target."}, {&a.RedisID, "The Redis target."}} {
		n.Describe(v.p, v.d)
	}
}

func (r Mount) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[MountArgs], error) {
	in, failures, err := infer.DefaultCheck[MountArgs](ctx, req.NewInputs)
	if err != nil {
		return infer.CheckResponse[MountArgs]{Inputs: in, Failures: failures}, err
	}
	typeComputed := req.NewInputs.Get("type").HasComputed()
	if !typeComputed && in.Type != mountTypeBind && in.Type != mountTypeVolume && in.Type != mountTypeFile {
		failures = append(failures, p.CheckFailure{Property: "type", Reason: "type must be one of bind, volume, file"})
	}
	if in.MountPath == "" && !req.NewInputs.Get("mountPath").HasComputed() {
		failures = append(failures, p.CheckFailure{Property: "mountPath", Reason: "mountPath must not be empty"})
	}
	computed := false
	for _, name := range []string{"applicationId", "composeId", "postgresId", "mysqlId", "mariadbId", "redisId"} {
		computed = computed || req.NewInputs.Get(name).HasComputed()
	}
	if !computed {
		if _, e := mountTargetFor(in); e != nil {
			failures = append(failures, p.CheckFailure{Property: "target", Reason: e.Error()})
		}
	}
	if typeComputed {
		return infer.CheckResponse[MountArgs]{Inputs: in, Failures: failures}, nil
	}
	provided := func(name string) bool {
		_, ok := req.NewInputs.AsMap()[name]
		return ok && !req.NewInputs.Get(name).HasComputed()
	}
	wrongFields := map[string][]string{
		mountTypeBind:   {"volumeName", "filePath", "content"},
		mountTypeVolume: {"hostPath", "filePath", "content"},
		mountTypeFile:   {"hostPath", "volumeName"},
	}
	for _, name := range wrongFields[in.Type] {
		if provided(name) {
			failures = append(failures, p.CheckFailure{Property: name, Reason: fmt.Sprintf("%s must not be set for %s mounts", name, in.Type)})
		}
	}
	switch in.Type {
	case mountTypeBind:
		if in.HostPath == nil && !req.NewInputs.Get("hostPath").HasComputed() {
			failures = append(failures, p.CheckFailure{Property: "hostPath", Reason: "hostPath must be set for bind mounts"})
		} else if in.HostPath != nil && *in.HostPath == "" && !req.NewInputs.Get("hostPath").HasComputed() {
			failures = append(failures, p.CheckFailure{Property: "hostPath", Reason: "hostPath must not be empty"})
		}
	case mountTypeVolume:
		if in.VolumeName == nil && !req.NewInputs.Get("volumeName").HasComputed() {
			failures = append(failures, p.CheckFailure{Property: "volumeName", Reason: "volumeName must be set for volume mounts"})
		} else if in.VolumeName != nil && *in.VolumeName == "" && !req.NewInputs.Get("volumeName").HasComputed() {
			failures = append(failures, p.CheckFailure{Property: "volumeName", Reason: "volumeName must not be empty"})
		}
	case mountTypeFile:
		if in.FilePath == nil && !req.NewInputs.Get("filePath").HasComputed() {
			failures = append(failures, p.CheckFailure{Property: "filePath", Reason: "filePath must be set for file mounts"})
		} else if in.FilePath != nil && *in.FilePath == "" && !req.NewInputs.Get("filePath").HasComputed() {
			failures = append(failures, p.CheckFailure{Property: "filePath", Reason: "filePath must not be empty"})
		}
		if in.Content == nil && !req.NewInputs.Get("content").HasComputed() {
			failures = append(failures, p.CheckFailure{Property: "content", Reason: "content must be set for file mounts"})
		}
	}
	return infer.CheckResponse[MountArgs]{Inputs: in, Failures: failures}, nil
}
func (r Mount) Diff(_ context.Context, req infer.DiffRequest[MountArgs, MountState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	fields := []struct {
		name             string
		changed, replace bool
	}{{"type", req.Inputs.Type != req.State.Type, true}, {"mountPath", req.Inputs.MountPath != req.State.MountPath, false}, {"hostPath", !sameOptionalString(req.Inputs.HostPath, req.State.HostPath), false}, {"volumeName", !sameOptionalString(req.Inputs.VolumeName, req.State.VolumeName), false}, {"filePath", !sameOptionalString(req.Inputs.FilePath, req.State.FilePath), false}, {"content", !sameOptionalString(req.Inputs.Content, req.State.Content), false}, {"applicationId", !sameOptionalString(req.Inputs.ApplicationID, req.State.ApplicationID), true}, {"composeId", !sameOptionalString(req.Inputs.ComposeID, req.State.ComposeID), true}, {"postgresId", !sameOptionalString(req.Inputs.PostgresID, req.State.PostgresID), true}, {"mysqlId", !sameOptionalString(req.Inputs.MySQLID, req.State.MySQLID), true}, {"mariadbId", !sameOptionalString(req.Inputs.MariaDBID, req.State.MariaDBID), true}, {"redisId", !sameOptionalString(req.Inputs.RedisID, req.State.RedisID), true}}
	for _, f := range fields {
		if f.changed {
			k := p.Update
			if f.replace {
				k = p.UpdateReplace
			}
			d[f.name] = p.PropertyDiff{Kind: k}
		}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d, DeleteBeforeReplace: hasReplacement(d)}, nil
}
func hasReplacement(d map[string]p.PropertyDiff) bool {
	for _, v := range d {
		if v.Kind == p.UpdateReplace {
			return true
		}
	}
	return false
}
func mountNullable(v *string) nullable.Nullable[string] {
	if v == nil {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(*v)
}
func mountCreateBody(a MountArgs, t mountTarget) generated.MountsCreateJSONRequestBody {
	return generated.MountsCreateJSONRequestBody{MountPath: a.MountPath, ServiceId: t.serviceID, ServiceType: ptr(generated.MountsCreateJSONBodyServiceType(t.serviceType)), Type: generated.MountsCreateJSONBodyType(a.Type), HostPath: mountNullable(a.HostPath), VolumeName: mountNullable(a.VolumeName), FilePath: mountNullable(a.FilePath), Content: mountNullable(a.Content)}
}
func mountUpdateBody(id string, a MountArgs, t mountTarget) generated.MountsUpdateJSONRequestBody {
	return generated.MountsUpdateJSONRequestBody{MountId: id, MountPath: &a.MountPath, ServiceType: ptr(generated.MountsUpdateJSONBodyServiceType(t.serviceType)), Type: ptr(generated.MountsUpdateJSONBodyType(a.Type)), HostPath: mountNullable(a.HostPath), VolumeName: mountNullable(a.VolumeName), FilePath: mountNullable(a.FilePath), Content: mountNullable(a.Content), ApplicationId: mountNullable(a.ApplicationID), ComposeId: mountNullable(a.ComposeID), PostgresId: mountNullable(a.PostgresID), MysqlId: mountNullable(a.MySQLID), MariadbId: mountNullable(a.MariaDBID), RedisId: mountNullable(a.RedisID)}
}
func nullablePointer(v nullable.Nullable[string]) *string {
	if !v.IsSpecified() || v.IsNull() {
		return nil
	}
	x := v.MustGet()
	return &x
}

func nullablePointerPreserving(v nullable.Nullable[string], prior *string) *string {
	if !v.IsSpecified() {
		return prior
	}
	return nullablePointer(v)
}

func sanitizeMountError(err error, args MountArgs, prior ...MountArgs) error {
	if err == nil {
		return err
	}
	secrets := make([]string, 0, 1+len(prior))
	if args.Content != nil {
		secrets = append(secrets, *args.Content)
	}
	for _, old := range prior {
		if old.Content != nil {
			secrets = append(secrets, *old.Content)
		}
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		copy := *apiErr
		for _, secret := range secrets {
			if secret != "" {
				copy.Code = strings.ReplaceAll(copy.Code, secret, "[REDACTED]")
				copy.Message = strings.ReplaceAll(copy.Message, secret, "[REDACTED]")
			}
		}
		return &copy
	}
	return sanitizeError(err, secrets...)
}
func mountArgsFrom(m *generated.Mount, prior MountArgs) (MountArgs, error) {
	a := prior
	if m.Type == nil || (*m.Type != mountTypeBind && *m.Type != mountTypeVolume && *m.Type != mountTypeFile) {
		return MountArgs{}, fmt.Errorf("mounts.one returned unsupported type %q", value(m.Type))
	}
	a.Type = *m.Type
	a.MountPath = value(m.MountPath)
	a.HostPath = nullablePointer(m.HostPath)
	a.VolumeName = nullablePointer(m.VolumeName)
	a.FilePath = nullablePointer(m.FilePath)
	a.Content = nullablePointerPreserving(m.Content, prior.Content)
	a.ApplicationID = nullablePointer(m.ApplicationId)
	a.ComposeID = nullablePointer(m.ComposeId)
	a.PostgresID = nullablePointer(m.PostgresId)
	a.MySQLID = nullablePointer(m.MysqlId)
	a.MariaDBID = nullablePointer(m.MariadbId)
	a.RedisID = nullablePointer(m.RedisId)
	ids := map[string]*string{"application": a.ApplicationID, "compose": a.ComposeID, "postgres": a.PostgresID, "mysql": a.MySQLID, "mariadb": a.MariaDBID, "redis": a.RedisID}
	if m.ServiceType == nil {
		return MountArgs{}, fmt.Errorf("mounts.one returned missing serviceType")
	}
	selected, ok := ids[*m.ServiceType]
	if !ok {
		return MountArgs{}, fmt.Errorf("mounts.one returned unsupported serviceType %q", *m.ServiceType)
	}
	count := 0
	for _, id := range ids {
		if id != nil && *id != "" {
			count++
		}
	}
	if count != 1 || selected == nil || *selected == "" {
		return MountArgs{}, fmt.Errorf("mounts.one returned ambiguous target IDs")
	}
	return a, nil
}
func (r Mount) read(ctx context.Context, id string, prior MountArgs) (MountState, error) {
	resp, err := r.client(ctx).MountsOneWithResponse(ctx, &generated.MountsOneParams{MountId: id})
	if err != nil {
		return MountState{}, sanitizeMountError(err, prior)
	}
	if resp.JSON200 == nil || resp.JSON200.MountId == "" {
		return MountState{}, sanitizeMountError(fmt.Errorf("mounts.one returned incomplete mount"), prior)
	}
	a, err := mountArgsFrom(resp.JSON200, prior)
	if err != nil {
		return MountState{}, sanitizeMountError(err, prior)
	}
	return MountState{MountArgs: a, MountID: resp.JSON200.MountId}, nil
}
func (r Mount) Create(ctx context.Context, req infer.CreateRequest[MountArgs]) (infer.CreateResponse[MountState], error) {
	state := MountState{MountArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[MountState]{Output: state}, nil
	}
	t, err := mountTargetFor(req.Inputs)
	if err != nil {
		return infer.CreateResponse[MountState]{}, err
	}
	resp, err := r.client(ctx).MountsCreateWithResponse(ctx, mountCreateBody(req.Inputs, t))
	if err != nil {
		return infer.CreateResponse[MountState]{}, sanitizeMountError(err, req.Inputs)
	}
	if resp.JSON200 == nil || resp.JSON200.MountId == "" {
		return infer.CreateResponse[MountState]{}, fmt.Errorf("mounts.create returned incomplete mount")
	}
	state.MountID = resp.JSON200.MountId
	readState, readErr := r.read(ctx, state.MountID, req.Inputs)
	if readErr == nil {
		state = readState
		_, err = deployMountTarget(ctx, r.client(ctx), t)
	} else {
		err = readErr
	}
	if err != nil {
		return infer.CreateResponse[MountState]{ID: state.MountID, Output: state}, initFailed(sanitizeMountError(err, req.Inputs))
	}
	return infer.CreateResponse[MountState]{ID: state.MountID, Output: state}, nil
}
func (r Mount) Read(ctx context.Context, req infer.ReadRequest[MountArgs, MountState]) (infer.ReadResponse[MountArgs, MountState], error) {
	s, err := r.read(ctx, req.ID, req.State.MountArgs)
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[MountArgs, MountState]{ID: ""}, nil
		}
		return infer.ReadResponse[MountArgs, MountState]{}, sanitizeMountError(err, req.Inputs, req.State.MountArgs)
	}
	return infer.ReadResponse[MountArgs, MountState]{ID: s.MountID, Inputs: s.MountArgs, State: s}, nil
}
func (r Mount) Update(ctx context.Context, req infer.UpdateRequest[MountArgs, MountState]) (infer.UpdateResponse[MountState], error) {
	s := MountState{MountArgs: req.Inputs, MountID: req.ID}
	if req.DryRun {
		return infer.UpdateResponse[MountState]{Output: s}, nil
	}
	t, err := mountTargetFor(req.Inputs)
	if err == nil {
		_, err = r.client(ctx).MountsUpdateWithResponse(ctx, mountUpdateBody(req.ID, req.Inputs, t))
		if err == nil {
			readState, readErr := r.read(ctx, req.ID, req.Inputs)
			if readErr != nil {
				err = readErr
			} else {
				s = readState
			}
		}
		if err == nil {
			_, err = deployMountTarget(ctx, r.client(ctx), t)
		}
	}
	if err != nil {
		return infer.UpdateResponse[MountState]{Output: s}, sanitizeMountError(err, req.Inputs, req.State.MountArgs)
	}
	return infer.UpdateResponse[MountState]{Output: s}, nil
}
func (r Mount) Delete(ctx context.Context, req infer.DeleteRequest[MountState]) (infer.DeleteResponse, error) {
	api := r.client(ctx)
	retained := req.State.MountArgs
	_, oneErr := api.MountsOneWithResponse(ctx, &generated.MountsOneParams{MountId: req.ID})
	if oneErr != nil && !client.IsNotFound(oneErr) {
		return infer.DeleteResponse{}, sanitizeMountError(oneErr, retained)
	}
	if oneErr == nil {
		if _, err := api.MountsRemoveWithResponse(ctx, generated.MountsRemoveJSONRequestBody{MountId: req.ID}); err != nil && !client.IsNotFound(err) {
			return infer.DeleteResponse{}, sanitizeMountError(err, retained)
		}
	}
	t, err := mountTargetFor(req.State.MountArgs)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	_, err = deployMountTarget(ctx, api, t)
	return infer.DeleteResponse{}, sanitizeMountError(err, retained)
}
func (r Mount) WireDependencies(f infer.FieldSelector, a *MountArgs, s *MountState) {
	deps := []infer.InputField{f.InputField(&a.Type), f.InputField(&a.MountPath), f.InputField(&a.HostPath), f.InputField(&a.VolumeName), f.InputField(&a.FilePath), f.InputField(&a.Content), f.InputField(&a.ApplicationID), f.InputField(&a.ComposeID), f.InputField(&a.PostgresID), f.InputField(&a.MySQLID), f.InputField(&a.MariaDBID), f.InputField(&a.RedisID)}
	f.OutputField(&s.MountID).DependsOn(deps...)
}
