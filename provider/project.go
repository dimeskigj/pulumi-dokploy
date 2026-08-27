package dokploy

import (
	"context"
	"fmt"

	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client"
	"github.com/gjorgjidimeski/pulumi-dokploy/internal/client/generated"
	"github.com/oapi-codegen/nullable"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type ProjectArgs struct {
	Name        string  `pulumi:"name"`
	Description *string `pulumi:"description,optional"`
}

type ProjectState struct {
	ProjectArgs
	ProjectID            string `pulumi:"projectId"`
	DefaultEnvironmentID string `pulumi:"defaultEnvironmentId"`
}

type Project struct{ client clientFactory }

func (r Project) Annotate(a infer.Annotator) { a.SetToken("index", "Project") }

func (r Project) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[ProjectArgs], error) {
	inputs, failures, err := infer.DefaultCheck[ProjectArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[ProjectArgs]{Inputs: inputs, Failures: failures}, err
	}
	if inputs.Name == "" {
		failures = append(failures, p.CheckFailure{Property: "name", Reason: "name must not be empty"})
	}
	return infer.CheckResponse[ProjectArgs]{Inputs: inputs, Failures: failures}, nil
}

func (r Project) Diff(context.Context, infer.DiffRequest[ProjectArgs, ProjectState]) (infer.DiffResponse, error) {
	return infer.DiffResponse{HasChanges: true, DetailedDiff: map[string]p.PropertyDiff{
		"name":        {Kind: p.Update},
		"description": {Kind: p.Update},
	}}, nil
}

func (r Project) Create(ctx context.Context, req infer.CreateRequest[ProjectArgs]) (infer.CreateResponse[ProjectState], error) {
	if req.DryRun {
		return infer.CreateResponse[ProjectState]{Output: ProjectState{ProjectArgs: req.Inputs}}, nil
	}
	c := r.client(ctx)
	body := generated.ProjectCreateJSONRequestBody{Name: req.Inputs.Name}
	if req.Inputs.Description != nil {
		body.Description = nullable.NewNullableWithValue(*req.Inputs.Description)
	}
	response, err := c.ProjectCreateWithResponse(ctx, body)
	if err != nil {
		return infer.CreateResponse[ProjectState]{}, err
	}
	if response.JSON200 == nil || response.JSON200.Project.ProjectId == nil || response.JSON200.Environment.EnvironmentId == nil {
		return infer.CreateResponse[ProjectState]{}, fmt.Errorf("project.create returned incomplete project")
	}
	state := ProjectState{ProjectArgs: req.Inputs, ProjectID: *response.JSON200.Project.ProjectId, DefaultEnvironmentID: *response.JSON200.Environment.EnvironmentId}
	return infer.CreateResponse[ProjectState]{ID: state.ProjectID, Output: state}, nil
}

func (r Project) Read(ctx context.Context, req infer.ReadRequest[ProjectArgs, ProjectState]) (infer.ReadResponse[ProjectArgs, ProjectState], error) {
	response, err := r.client(ctx).ProjectOneWithResponse(ctx, &generated.ProjectOneParams{ProjectId: req.ID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[ProjectArgs, ProjectState]{ID: ""}, nil
		}
		return infer.ReadResponse[ProjectArgs, ProjectState]{}, err
	}
	if response.JSON200 == nil || response.JSON200.ProjectId == nil {
		return infer.ReadResponse[ProjectArgs, ProjectState]{}, fmt.Errorf("project.one returned incomplete project")
	}
	project := response.JSON200
	args := ProjectArgs{Name: value(project.Name), Description: project.Description}
	state := ProjectState{ProjectArgs: args, ProjectID: *project.ProjectId, DefaultEnvironmentID: value(project.DefaultEnvironmentId)}
	return infer.ReadResponse[ProjectArgs, ProjectState]{ID: *project.ProjectId, Inputs: args, State: state}, nil
}

func (r Project) Update(ctx context.Context, req infer.UpdateRequest[ProjectArgs, ProjectState]) (infer.UpdateResponse[ProjectState], error) {
	if req.DryRun {
		return infer.UpdateResponse[ProjectState]{Output: ProjectState{ProjectArgs: req.Inputs, ProjectID: req.State.ProjectID, DefaultEnvironmentID: req.State.DefaultEnvironmentID}}, nil
	}
	body := generated.ProjectUpdateJSONRequestBody{ProjectId: req.ID, Name: &req.Inputs.Name}
	if req.Inputs.Description != nil {
		body.Description = nullable.NewNullableWithValue(*req.Inputs.Description)
	}
	if _, err := r.client(ctx).ProjectUpdateWithResponse(ctx, body); err != nil {
		return infer.UpdateResponse[ProjectState]{}, err
	}
	return infer.UpdateResponse[ProjectState]{Output: ProjectState{ProjectArgs: req.Inputs, ProjectID: req.ID, DefaultEnvironmentID: req.State.DefaultEnvironmentID}}, nil
}

func (r Project) Delete(ctx context.Context, req infer.DeleteRequest[ProjectState]) (infer.DeleteResponse, error) {
	_, err := r.client(ctx).ProjectRemoveWithResponse(ctx, generated.ProjectRemoveJSONRequestBody{ProjectId: req.ID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, err
}

func (r Project) WireDependencies(f infer.FieldSelector, args *ProjectArgs, state *ProjectState) {
	f.OutputField(&state.ProjectID).DependsOn(f.InputField(&args.Name), f.InputField(&args.Description))
	f.OutputField(&state.DefaultEnvironmentID).DependsOn(f.InputField(&args.Name), f.InputField(&args.Description))
}

func value(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
