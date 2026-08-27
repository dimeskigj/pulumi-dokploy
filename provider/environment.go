package dokploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client"
	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client/generated"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

const unsupportedDefaultEnvironment = "default environments are managed by Project and cannot be managed as standalone Environment resources"

type EnvironmentArgs struct {
	ProjectID   string  `pulumi:"projectId"`
	Name        string  `pulumi:"name"`
	Description *string `pulumi:"description,optional"`
}
type EnvironmentState struct {
	EnvironmentArgs
	EnvironmentID string `pulumi:"environmentId"`
	IsDefault     bool   `pulumi:"isDefault"`
}
type Environment struct{ client clientFactory }

func (r Environment) Annotate(a infer.Annotator) { a.SetToken("index", "Environment") }

func (r Environment) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[EnvironmentArgs], error) {
	inputs, failures, err := infer.DefaultCheck[EnvironmentArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[EnvironmentArgs]{Inputs: inputs, Failures: failures}, err
	}
	if inputs.Name == "" {
		failures = append(failures, p.CheckFailure{Property: "name", Reason: "name must not be empty"})
	}
	if inputs.Name == "production" {
		failures = append(failures, p.CheckFailure{Property: "name", Reason: "production is the default environment and is not supported as a standalone Environment"})
	}
	return infer.CheckResponse[EnvironmentArgs]{Inputs: inputs, Failures: failures}, nil
}

func (r Environment) Diff(context.Context, infer.DiffRequest[EnvironmentArgs, EnvironmentState]) (infer.DiffResponse, error) {
	return infer.DiffResponse{HasChanges: true, DetailedDiff: map[string]p.PropertyDiff{"projectId": {Kind: p.UpdateReplace}, "name": {Kind: p.Update}, "description": {Kind: p.Update}}}, nil
}

func (r Environment) Create(ctx context.Context, req infer.CreateRequest[EnvironmentArgs]) (infer.CreateResponse[EnvironmentState], error) {
	if req.DryRun {
		return infer.CreateResponse[EnvironmentState]{Output: EnvironmentState{EnvironmentArgs: req.Inputs}}, nil
	}
	body := generated.EnvironmentCreateJSONRequestBody{ProjectId: req.Inputs.ProjectID, Name: req.Inputs.Name, Description: req.Inputs.Description}
	response, err := r.client(ctx).EnvironmentCreateWithResponse(ctx, body)
	if err != nil {
		return infer.CreateResponse[EnvironmentState]{}, err
	}
	if response.JSON200 == nil || response.JSON200.EnvironmentId == nil {
		return infer.CreateResponse[EnvironmentState]{}, fmt.Errorf("environment.create returned incomplete environment")
	}
	isDefault, err := environmentDefault(response.Body)
	if err != nil {
		return infer.CreateResponse[EnvironmentState]{}, err
	}
	if isDefault {
		return infer.CreateResponse[EnvironmentState]{}, errors.New(unsupportedDefaultEnvironment)
	}
	state := EnvironmentState{EnvironmentArgs: req.Inputs, EnvironmentID: *response.JSON200.EnvironmentId, IsDefault: isDefault}
	return infer.CreateResponse[EnvironmentState]{ID: state.EnvironmentID, Output: state}, nil
}

func (r Environment) Read(ctx context.Context, req infer.ReadRequest[EnvironmentArgs, EnvironmentState]) (infer.ReadResponse[EnvironmentArgs, EnvironmentState], error) {
	response, err := r.client(ctx).EnvironmentOneWithResponse(ctx, &generated.EnvironmentOneParams{EnvironmentId: req.ID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[EnvironmentArgs, EnvironmentState]{ID: ""}, nil
		}
		return infer.ReadResponse[EnvironmentArgs, EnvironmentState]{}, err
	}
	if response.JSON200 == nil || response.JSON200.EnvironmentId == nil {
		return infer.ReadResponse[EnvironmentArgs, EnvironmentState]{}, fmt.Errorf("environment.one returned incomplete environment")
	}
	isDefault, err := environmentDefault(response.Body)
	if err != nil {
		return infer.ReadResponse[EnvironmentArgs, EnvironmentState]{}, err
	}
	if isDefault {
		return infer.ReadResponse[EnvironmentArgs, EnvironmentState]{}, errors.New(unsupportedDefaultEnvironment)
	}
	e := response.JSON200
	args := EnvironmentArgs{ProjectID: value(e.ProjectId), Name: value(e.Name), Description: e.Description}
	state := EnvironmentState{EnvironmentArgs: args, EnvironmentID: *e.EnvironmentId, IsDefault: isDefault}
	return infer.ReadResponse[EnvironmentArgs, EnvironmentState]{ID: *e.EnvironmentId, Inputs: args, State: state}, nil
}

func (r Environment) Update(ctx context.Context, req infer.UpdateRequest[EnvironmentArgs, EnvironmentState]) (infer.UpdateResponse[EnvironmentState], error) {
	if req.DryRun {
		return infer.UpdateResponse[EnvironmentState]{Output: EnvironmentState{EnvironmentArgs: req.Inputs, EnvironmentID: req.State.EnvironmentID, IsDefault: req.State.IsDefault}}, nil
	}
	body := generated.EnvironmentUpdateJSONRequestBody{EnvironmentId: req.ID, Name: &req.Inputs.Name, Description: req.Inputs.Description}
	if _, err := r.client(ctx).EnvironmentUpdateWithResponse(ctx, body); err != nil {
		return infer.UpdateResponse[EnvironmentState]{}, err
	}
	return infer.UpdateResponse[EnvironmentState]{Output: EnvironmentState{EnvironmentArgs: req.Inputs, EnvironmentID: req.ID, IsDefault: req.State.IsDefault}}, nil
}

func (r Environment) Delete(ctx context.Context, req infer.DeleteRequest[EnvironmentState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).EnvironmentRemoveWithResponse(ctx, generated.EnvironmentRemoveJSONRequestBody{EnvironmentId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, err
}
func (r Environment) WireDependencies(f infer.FieldSelector, args *EnvironmentArgs, state *EnvironmentState) {
	f.OutputField(&state.EnvironmentID).DependsOn(f.InputField(&args.ProjectID), f.InputField(&args.Name), f.InputField(&args.Description))
	f.OutputField(&state.IsDefault).DependsOn(f.InputField(&args.ProjectID), f.InputField(&args.Name), f.InputField(&args.Description))
}

func environmentDefault(body []byte) (bool, error) {
	var payload struct {
		IsDefault   bool `json:"isDefault"`
		Environment *struct {
			IsDefault bool `json:"isDefault"`
		} `json:"environment"`
	}
	if len(body) == 0 {
		return false, nil
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false, fmt.Errorf("decode environment response: %w", err)
	}
	if payload.Environment != nil {
		return payload.Environment.IsDefault, nil
	}
	return payload.IsDefault, nil
}
