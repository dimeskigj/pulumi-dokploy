package dokploy

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client"
	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type ApplicationArgs struct {
	Name          string            `pulumi:"name"`
	AppName       *string           `pulumi:"appName,optional"`
	Description   *string           `pulumi:"description,optional"`
	EnvironmentID string            `pulumi:"environmentId"`
	ServerID      *string           `pulumi:"serverId,optional"`
	Source        ApplicationSource `pulumi:"source"`
	Environment   *string           `pulumi:"environment,optional" provider:"secret"`
	BuildArgs     *string           `pulumi:"buildArgs,optional" provider:"secret"`
	BuildSecrets  *string           `pulumi:"buildSecrets,optional" provider:"secret"`
	CreateEnvFile bool              `pulumi:"createEnvFile,optional"`
}

type ApplicationState struct {
	ApplicationArgs
	ApplicationID string `pulumi:"applicationId"`
	Status        string `pulumi:"status"`
}

type Application struct{ client clientFactory }

func (r Application) Annotate(a infer.Annotator) { a.SetToken("index", "Application") }

func (r Application) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[ApplicationArgs], error) {
	inputs, failures, err := infer.DefaultCheck[ApplicationArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[ApplicationArgs]{Inputs: inputs, Failures: failures}, err
	}
	if inputs.Name == "" {
		failures = append(failures, p.CheckFailure{Property: "name", Reason: "name must not be empty"})
	}
	if inputs.EnvironmentID == "" {
		failures = append(failures, p.CheckFailure{Property: "environmentId", Reason: "environmentId must not be empty"})
	}
	if err := inputs.Source.validate(); err != nil {
		failures = append(failures, p.CheckFailure{Property: "source", Reason: err.Error()})
	}
	return infer.CheckResponse[ApplicationArgs]{Inputs: inputs, Failures: failures}, nil
}

func (r Application) Diff(_ context.Context, req infer.DiffRequest[ApplicationArgs, ApplicationState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	if req.Inputs.EnvironmentID != req.State.EnvironmentID {
		d["environmentId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if !sameOptionalString(req.Inputs.ServerID, req.State.ServerID) {
		d["serverId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if req.Inputs.Source.Type != req.State.Source.Type {
		d["source.type"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if req.Inputs.Name != req.State.Name {
		d["name"] = p.PropertyDiff{Kind: p.Update}
	}
	if !sameOptionalString(req.Inputs.AppName, req.State.AppName) {
		d["appName"] = p.PropertyDiff{Kind: p.Update}
	}
	if !sameOptionalString(req.Inputs.Description, req.State.Description) {
		d["description"] = p.PropertyDiff{Kind: p.Update}
	}
	if !reflect.DeepEqual(req.Inputs.Source, req.State.Source) && req.Inputs.Source.Type == req.State.Source.Type {
		d["source"] = p.PropertyDiff{Kind: p.Update}
	}
	if !sameOptionalString(req.Inputs.Environment, req.State.Environment) {
		d["environment"] = p.PropertyDiff{Kind: p.Update}
	}
	if !sameOptionalString(req.Inputs.BuildArgs, req.State.BuildArgs) {
		d["buildArgs"] = p.PropertyDiff{Kind: p.Update}
	}
	if !sameOptionalString(req.Inputs.BuildSecrets, req.State.BuildSecrets) {
		d["buildSecrets"] = p.PropertyDiff{Kind: p.Update}
	}
	if req.Inputs.CreateEnvFile != req.State.CreateEnvFile {
		d["createEnvFile"] = p.PropertyDiff{Kind: p.Update}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r Application) Create(ctx context.Context, req infer.CreateRequest[ApplicationArgs]) (infer.CreateResponse[ApplicationState], error) {
	state := ApplicationState{ApplicationArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[ApplicationState]{Output: state}, nil
	}
	api := r.client(ctx)
	body := generated.ApplicationCreateJSONRequestBody{Name: req.Inputs.Name, EnvironmentId: req.Inputs.EnvironmentID}
	if req.Inputs.AppName != nil {
		body.AppName = req.Inputs.AppName
	}
	if req.Inputs.Description != nil {
		body.Description = nullable.NewNullableWithValue(*req.Inputs.Description)
	}
	if req.Inputs.ServerID != nil {
		body.ServerId = nullable.NewNullableWithValue(*req.Inputs.ServerID)
	}
	response, err := api.ApplicationCreateWithResponse(ctx, body)
	if err != nil {
		return infer.CreateResponse[ApplicationState]{}, err
	}
	if response.JSON200 == nil || response.JSON200.ApplicationId == nil {
		return infer.CreateResponse[ApplicationState]{}, fmt.Errorf("application.create returned incomplete application")
	}
	state.ApplicationID = *response.JSON200.ApplicationId
	failSetup := func(setupErr error) (infer.CreateResponse[ApplicationState], error) {
		if _, cleanupErr := api.ApplicationDeleteWithResponse(ctx, generated.ApplicationDeleteJSONRequestBody{ApplicationId: state.ApplicationID}); cleanupErr != nil {
			setupErr = fmt.Errorf("%w (cleanup failed: %v)", setupErr, cleanupErr)
		}
		return infer.CreateResponse[ApplicationState]{ID: state.ApplicationID, Output: state}, setupErr
	}
	if err := configureApplicationSource(ctx, api, state.ApplicationID, req.Inputs.Source); err != nil {
		return failSetup(err)
	}
	if err := configureApplicationBuild(ctx, api, state.ApplicationID, req.Inputs.Source); err != nil {
		return failSetup(err)
	}
	if err := configureApplicationEnvironment(ctx, api, state.ApplicationID, req.Inputs); err != nil {
		return failSetup(err)
	}
	if _, err := api.ApplicationDeployWithResponse(ctx, generated.ApplicationDeployJSONRequestBody{ApplicationId: state.ApplicationID}); err != nil {
		return infer.CreateResponse[ApplicationState]{ID: state.ApplicationID, Output: state}, initFailed(err)
	}
	if err := waitForDone(ctx, "application", state.ApplicationID, func(ctx context.Context) (string, error) { return applicationStatus(ctx, api, state.ApplicationID) }); err != nil {
		return infer.CreateResponse[ApplicationState]{ID: state.ApplicationID, Output: state}, initFailed(err)
	}
	state.Status = "done"
	return infer.CreateResponse[ApplicationState]{ID: state.ApplicationID, Output: state}, nil
}

func configureApplicationEnvironment(ctx context.Context, api *client.Client, id string, args ApplicationArgs) error {
	body := generated.ApplicationSaveEnvironmentJSONRequestBody{ApplicationId: id, CreateEnvFile: args.CreateEnvFile, Env: nullable.NewNullNullable[string](), BuildArgs: nullable.NewNullNullable[string](), BuildSecrets: nullable.NewNullNullable[string]()}
	if args.Environment != nil {
		body.Env = nullable.NewNullableWithValue(*args.Environment)
	}
	if args.BuildArgs != nil {
		body.BuildArgs = nullable.NewNullableWithValue(*args.BuildArgs)
	}
	if args.BuildSecrets != nil {
		body.BuildSecrets = nullable.NewNullableWithValue(*args.BuildSecrets)
	}
	_, err := api.ApplicationSaveEnvironmentWithResponse(ctx, body)
	return err
}

func applicationStatus(ctx context.Context, api *client.Client, id string) (string, error) {
	response, err := api.ApplicationOneWithResponse(ctx, &generated.ApplicationOneParams{ApplicationId: id})
	if err != nil {
		return "", err
	}
	if response.JSON200 == nil || response.JSON200.Status == nil {
		return "", fmt.Errorf("application.one returned incomplete application")
	}
	return *response.JSON200.Status, nil
}

func (r Application) Read(ctx context.Context, req infer.ReadRequest[ApplicationArgs, ApplicationState]) (infer.ReadResponse[ApplicationArgs, ApplicationState], error) {
	response, err := r.client(ctx).ApplicationOneWithResponse(ctx, &generated.ApplicationOneParams{ApplicationId: req.ID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[ApplicationArgs, ApplicationState]{ID: ""}, nil
		}
		return infer.ReadResponse[ApplicationArgs, ApplicationState]{}, err
	}
	if response.JSON200 == nil || response.JSON200.ApplicationId == nil {
		return infer.ReadResponse[ApplicationArgs, ApplicationState]{}, fmt.Errorf("application.one returned incomplete application")
	}
	a := response.JSON200
	args := req.State.ApplicationArgs
	args.Name, args.EnvironmentID = value(a.Name), value(a.EnvironmentId)
	args.AppName, args.Description, args.ServerID = a.AppName, a.Description, a.ServerId
	if a.Env != nil {
		args.Environment = a.Env
	}
	if a.BuildArgs != nil {
		args.BuildArgs = a.BuildArgs
	}
	if a.BuildSecrets != nil {
		args.BuildSecrets = a.BuildSecrets
	}
	if a.CreateEnvFile != nil {
		args.CreateEnvFile = *a.CreateEnvFile
	}
	state := ApplicationState{ApplicationArgs: args, ApplicationID: *a.ApplicationId, Status: value(a.Status)}
	return infer.ReadResponse[ApplicationArgs, ApplicationState]{ID: *a.ApplicationId, Inputs: args, State: state}, nil
}

func (r Application) Update(ctx context.Context, req infer.UpdateRequest[ApplicationArgs, ApplicationState]) (infer.UpdateResponse[ApplicationState], error) {
	state := ApplicationState{ApplicationArgs: req.Inputs, ApplicationID: req.ID, Status: req.State.Status}
	if req.DryRun {
		return infer.UpdateResponse[ApplicationState]{Output: state}, nil
	}
	body := generated.ApplicationUpdateJSONRequestBody{ApplicationId: req.ID, AppName: req.Inputs.AppName, Name: &req.Inputs.Name, Description: nullable.NewNullNullable[string](), EnvironmentId: &req.Inputs.EnvironmentID, CreateEnvFile: &req.Inputs.CreateEnvFile, SourceType: sourceType(req.Inputs.Source.Type), Env: nullable.NewNullNullable[string](), BuildArgs: nullable.NewNullNullable[string](), BuildSecrets: nullable.NewNullNullable[string]()}
	if req.Inputs.Description != nil {
		body.Description = nullable.NewNullableWithValue(*req.Inputs.Description)
	}
	if req.Inputs.Environment != nil {
		body.Env = nullable.NewNullableWithValue(*req.Inputs.Environment)
	}
	if req.Inputs.BuildArgs != nil {
		body.BuildArgs = nullable.NewNullableWithValue(*req.Inputs.BuildArgs)
	}
	if req.Inputs.BuildSecrets != nil {
		body.BuildSecrets = nullable.NewNullableWithValue(*req.Inputs.BuildSecrets)
	}
	if _, err := r.client(ctx).ApplicationUpdateWithResponse(ctx, body); err != nil {
		return infer.UpdateResponse[ApplicationState]{}, err
	}
	runtimeChanged := !reflect.DeepEqual(req.Inputs.Source, req.State.Source) || !sameOptionalString(req.Inputs.Environment, req.State.Environment) || !sameOptionalString(req.Inputs.BuildArgs, req.State.BuildArgs) || !sameOptionalString(req.Inputs.BuildSecrets, req.State.BuildSecrets) || req.Inputs.CreateEnvFile != req.State.CreateEnvFile
	if runtimeChanged {
		if _, err := r.client(ctx).ApplicationRedeployWithResponse(ctx, generated.ApplicationRedeployJSONRequestBody{ApplicationId: req.ID}); err != nil {
			return infer.UpdateResponse[ApplicationState]{Output: state}, err
		}
		if err := waitForDone(ctx, "application", req.ID, func(ctx context.Context) (string, error) { return applicationStatus(ctx, r.client(ctx), req.ID) }); err != nil {
			return infer.UpdateResponse[ApplicationState]{Output: state}, err
		}
		state.Status = "done"
	}
	return infer.UpdateResponse[ApplicationState]{Output: state}, nil
}

func sourceType(t ApplicationSourceType) *generated.ApplicationUpdateJSONBodySourceType {
	v := generated.ApplicationUpdateJSONBodySourceType(t)
	return &v
}

func (r Application) Delete(ctx context.Context, req infer.DeleteRequest[ApplicationState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).ApplicationDeleteWithResponse(ctx, generated.ApplicationDeleteJSONRequestBody{ApplicationId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, err
}

func (r Application) WireDependencies(f infer.FieldSelector, args *ApplicationArgs, state *ApplicationState) {
	f.OutputField(&state.ApplicationID).DependsOn(f.InputField(&args.Name), f.InputField(&args.EnvironmentID), f.InputField(&args.ServerID), f.InputField(&args.Source), f.InputField(&args.Environment), f.InputField(&args.BuildArgs), f.InputField(&args.BuildSecrets))
	f.OutputField(&state.Status).DependsOn(f.InputField(&args.Name), f.InputField(&args.EnvironmentID), f.InputField(&args.ServerID), f.InputField(&args.Source), f.InputField(&args.Environment), f.InputField(&args.BuildArgs), f.InputField(&args.BuildSecrets))
}
