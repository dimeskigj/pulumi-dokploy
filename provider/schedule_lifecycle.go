package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"
	"github.com/pulumi/pulumi-go-provider/infer"
)

func scheduleNullable(v *string, clear bool) nullable.Nullable[string] {
	if v != nil {
		return nullable.NewNullableWithValue(*v)
	}
	if clear {
		return nullable.NewNullNullable[string]()
	}
	return nil
}

func scheduleCreateBody(id string, a ScheduleArgs) generated.ScheduleCreateJSONRequestBody {
	typ := generated.ScheduleCreateJSONBodyScheduleType(a.ScheduleType)
	b := generated.ScheduleCreateJSONRequestBody{
		ScheduleId: &id, Name: a.Name, Command: a.Command, CronExpression: a.CronExpression,
		ScheduleType: &typ, Enabled: &a.Enabled, AppName: a.AppName,
		ApplicationId: scheduleNullable(a.ApplicationID, false), ComposeId: scheduleNullable(a.ComposeID, false),
		ServerId: scheduleNullable(a.ServerID, false), Description: scheduleNullable(a.Description, false),
		Script: scheduleNullable(a.Script, false), Timezone: scheduleNullable(a.Timezone, false),
		OrganizationId: scheduleNullable(a.OrganizationID, false), ServiceName: scheduleNullable(a.ServiceName, false),
	}
	if a.ShellType != nil {
		v := generated.ScheduleCreateJSONBodyShellType(*a.ShellType)
		b.ShellType = &v
	}
	return b
}

func scheduleUpdateBody(id string, a ScheduleArgs) generated.ScheduleUpdateJSONRequestBody {
	typ := generated.ScheduleUpdateJSONBodyScheduleType(a.ScheduleType)
	b := generated.ScheduleUpdateJSONRequestBody{
		ScheduleId: id, Name: a.Name, Command: a.Command, CronExpression: a.CronExpression,
		ScheduleType: &typ, Enabled: &a.Enabled, AppName: a.AppName,
		ApplicationId: scheduleNullable(a.ApplicationID, true), ComposeId: scheduleNullable(a.ComposeID, true),
		ServerId: scheduleNullable(a.ServerID, true), Description: scheduleNullable(a.Description, true),
		Script: scheduleNullable(a.Script, true), Timezone: scheduleNullable(a.Timezone, true),
		OrganizationId: scheduleNullable(a.OrganizationID, true), ServiceName: scheduleNullable(a.ServiceName, true),
	}
	if a.ShellType != nil {
		v := generated.ScheduleUpdateJSONBodyShellType(*a.ShellType)
		b.ShellType = &v
	}
	return b
}

func scheduleObservedNullable(v nullable.Nullable[string], prior *string) *string {
	if !v.IsSpecified() {
		return prior
	}
	if v.IsNull() {
		return nil
	}
	return ptr(v.MustGet())
}

func scheduleArgsFrom(v *generated.Schedule, prior ScheduleArgs) (ScheduleArgs, error) {
	if v == nil || v.ScheduleId == "" || v.Name == nil || *v.Name == "" ||
		v.Command == nil || *v.Command == "" || v.CronExpression == nil || *v.CronExpression == "" ||
		v.ScheduleType == nil || !v.ScheduleType.Valid() {
		return ScheduleArgs{}, errors.New("schedule.one returned an incomplete schedule")
	}
	a := prior
	a.Name, a.Command, a.CronExpression, a.ScheduleType = *v.Name, *v.Command, *v.CronExpression, string(*v.ScheduleType)
	if v.Enabled != nil {
		a.Enabled = *v.Enabled
	}
	if v.AppName != nil {
		a.AppName = v.AppName
	}
	if v.ShellType != nil {
		if !v.ShellType.Valid() {
			return ScheduleArgs{}, errors.New("schedule.one returned an invalid shell type")
		}
		a.ShellType = ptr(string(*v.ShellType))
	}
	a.ApplicationID = scheduleObservedNullable(v.ApplicationId, a.ApplicationID)
	a.ComposeID = scheduleObservedNullable(v.ComposeId, a.ComposeID)
	a.ServerID = scheduleObservedNullable(v.ServerId, a.ServerID)
	a.Description = scheduleObservedNullable(v.Description, a.Description)
	a.Script = scheduleObservedNullable(v.Script, a.Script)
	a.Timezone = scheduleObservedNullable(v.Timezone, a.Timezone)
	a.OrganizationID = scheduleObservedNullable(v.OrganizationId, a.OrganizationID)
	a.ServiceName = scheduleObservedNullable(v.ServiceName, a.ServiceName)
	return a, nil
}

// Do not forward arbitrary server prose or transport URLs: even APIError fields
// may contain commands, scripts, credentials, or IDs not present in prior inputs.
// Keeping only classification is safer than substring redaction (empty, short,
// transformed, and server-observed secrets cannot be reliably matched).
func sanitizeScheduleError(err error, _ ScheduleArgs, _ ...ScheduleArgs) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return &client.APIError{StatusCode: apiErr.StatusCode}
	}
	return errors.New("schedule request failed; response details withheld")
}

func readSchedule(ctx context.Context, api *client.Client, id string, prior ScheduleArgs) (ScheduleState, error) {
	resp, err := api.ScheduleOneWithResponse(ctx, &generated.ScheduleOneParams{ScheduleId: id})
	if err != nil {
		return ScheduleState{}, err
	}
	if resp.JSON200 == nil || resp.JSON200.ScheduleId != id {
		return ScheduleState{}, errors.New("schedule.one could not confirm the requested schedule")
	}
	// The generated non-nullable optional pointers collapse JSON null and
	// omission. Inspect presence so invalid enabled:null is rejected and explicit
	// string clears do not retain prior values. Nullable fields retain presence.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(resp.Body, &fields); err != nil {
		return ScheduleState{}, errors.New("schedule.one returned an invalid schedule")
	}
	const jsonNull = "null"
	if string(fields["enabled"]) == jsonNull {
		return ScheduleState{}, errors.New("schedule.one returned an invalid enabled value")
	}
	if string(fields["appName"]) == jsonNull {
		prior.AppName = nil
	}
	if string(fields["shellType"]) == jsonNull {
		prior.ShellType = nil
	}
	a, err := scheduleArgsFrom(resp.JSON200, prior)
	if err != nil {
		return ScheduleState{}, err
	}
	return ScheduleState{ScheduleArgs: a, ScheduleID: id}, nil
}

func (r Schedule) Create(ctx context.Context, req infer.CreateRequest[ScheduleArgs]) (infer.CreateResponse[ScheduleState], error) {
	state := ScheduleState{ScheduleArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[ScheduleState]{Output: state}, nil
	}
	id := uuid.NewString()
	api := r.client(ctx)
	if _, err := api.ScheduleCreateWithResponse(ctx, scheduleCreateBody(id, req.Inputs)); err != nil {
		return infer.CreateResponse[ScheduleState]{}, fmt.Errorf("schedule.create failed: %w", sanitizeScheduleError(err, req.Inputs))
	}
	state.ScheduleID = id
	observed, err := readSchedule(ctx, api, id, req.Inputs)
	if err != nil {
		// An acknowledged creation is never retried or automatically deleted.
		// Persist the attempted identity without adopting any unrelated response.
		return infer.CreateResponse[ScheduleState]{ID: id, Output: state}, initFailed(fmt.Errorf("schedule.create was acknowledged but ownership could not be confirmed; inspect the partial resource state before retrying: %w", sanitizeScheduleError(err, req.Inputs)))
	}
	return infer.CreateResponse[ScheduleState]{ID: id, Output: observed}, nil
}

func (r Schedule) Read(ctx context.Context, req infer.ReadRequest[ScheduleArgs, ScheduleState]) (infer.ReadResponse[ScheduleArgs, ScheduleState], error) {
	state, err := readSchedule(ctx, r.client(ctx), req.ID, req.State.ScheduleArgs)
	if client.IsNotFound(err) {
		return infer.ReadResponse[ScheduleArgs, ScheduleState]{}, nil
	}
	if err != nil {
		return infer.ReadResponse[ScheduleArgs, ScheduleState]{}, fmt.Errorf("schedule.one failed: %w", sanitizeScheduleError(err, req.Inputs, req.State.ScheduleArgs))
	}
	return infer.ReadResponse[ScheduleArgs, ScheduleState]{ID: req.ID, Inputs: state.ScheduleArgs, State: state}, nil
}

func (r Schedule) Update(ctx context.Context, req infer.UpdateRequest[ScheduleArgs, ScheduleState]) (infer.UpdateResponse[ScheduleState], error) {
	state := ScheduleState{ScheduleArgs: req.Inputs, ScheduleID: req.ID}
	if req.DryRun {
		return infer.UpdateResponse[ScheduleState]{Output: state}, nil
	}
	api := r.client(ctx)
	if _, err := api.ScheduleUpdateWithResponse(ctx, scheduleUpdateBody(req.ID, req.Inputs)); err != nil {
		return infer.UpdateResponse[ScheduleState]{Output: state}, fmt.Errorf("schedule.update failed: %w", sanitizeScheduleError(err, req.Inputs, req.State.ScheduleArgs))
	}
	observed, err := readSchedule(ctx, api, req.ID, req.Inputs)
	if err != nil {
		return infer.UpdateResponse[ScheduleState]{Output: state}, fmt.Errorf("schedule.update was acknowledged but read-back failed: %w", sanitizeScheduleError(err, req.Inputs, req.State.ScheduleArgs))
	}
	return infer.UpdateResponse[ScheduleState]{Output: observed}, nil
}

func (r Schedule) Delete(ctx context.Context, req infer.DeleteRequest[ScheduleState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).ScheduleDeleteWithResponse(ctx, generated.ScheduleDeleteJSONRequestBody{ScheduleId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	if err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("schedule.delete failed: %w", sanitizeScheduleError(err, req.State.ScheduleArgs))
	}
	return infer.DeleteResponse{}, nil
}
