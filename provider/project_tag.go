package dokploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/dimeskigj/pulumi-dokploy/internal/client"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type ProjectTagArgs struct {
	ProjectID string `pulumi:"projectId" provider:"replaceOnChanges"`
	TagID     string `pulumi:"tagId" provider:"replaceOnChanges"`
}

type ProjectTagState struct{ ProjectTagArgs }

func (a *ProjectTagArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.ProjectID, "The Dokploy project ID.")
	n.Describe(&a.TagID, "The Dokploy tag ID.")
}
func (r *ProjectTag) Annotate(a infer.Annotator) {
	a.SetToken("index", "ProjectTag")
	a.Describe(&r, "A Dokploy project-to-tag association.")
}

type ProjectTag struct{ client clientFactory }

func formatProjectTagID(projectID, tagID string) string { return projectID + "/" + tagID }
func parseProjectTagID(id string) (string, string, error) {
	parts := strings.Split(id, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid project tag ID %q", id)
	}
	return parts[0], parts[1], nil
}

func (r ProjectTag) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[ProjectTagArgs], error) {
	in, failures, err := infer.DefaultCheck[ProjectTagArgs](ctx, req.NewInputs)
	if err != nil || len(failures) != 0 {
		return infer.CheckResponse[ProjectTagArgs]{Inputs: in, Failures: failures}, err
	}
	for _, f := range []struct{ name, value string }{{"projectId", in.ProjectID}, {"tagId", in.TagID}} {
		if f.value == "" && !req.NewInputs.Get(f.name).HasComputed() {
			failures = append(failures, p.CheckFailure{Property: f.name, Reason: f.name + " must not be empty"})
		}
	}
	return infer.CheckResponse[ProjectTagArgs]{Inputs: in, Failures: failures}, nil
}

func (r ProjectTag) Diff(_ context.Context, req infer.DiffRequest[ProjectTagArgs, ProjectTagState]) (infer.DiffResponse, error) {
	d := map[string]p.PropertyDiff{}
	if req.Inputs.ProjectID != req.State.ProjectID {
		d["projectId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	if req.Inputs.TagID != req.State.TagID {
		d["tagId"] = p.PropertyDiff{Kind: p.UpdateReplace}
	}
	return infer.DiffResponse{HasChanges: len(d) > 0, DetailedDiff: d}, nil
}

func (r ProjectTag) Create(ctx context.Context, req infer.CreateRequest[ProjectTagArgs]) (infer.CreateResponse[ProjectTagState], error) {
	state := ProjectTagState{ProjectTagArgs: req.Inputs}
	if req.DryRun {
		return infer.CreateResponse[ProjectTagState]{Output: state}, nil
	}
	_, err := r.client(ctx).TagAssignToProjectWithResponse(ctx, generated.TagAssignToProjectJSONRequestBody{ProjectId: req.Inputs.ProjectID, TagId: req.Inputs.TagID})
	if err != nil {
		return infer.CreateResponse[ProjectTagState]{}, err
	}
	id := formatProjectTagID(req.Inputs.ProjectID, req.Inputs.TagID)
	read, err := r.Read(ctx, infer.ReadRequest[ProjectTagArgs, ProjectTagState]{ID: id, State: state})
	if err != nil {
		return infer.CreateResponse[ProjectTagState]{ID: id, Output: state}, initFailed(err)
	}
	if read.ID == "" {
		return infer.CreateResponse[ProjectTagState]{ID: id, Output: state}, initFailed(fmt.Errorf("project.one returned association missing after create"))
	}
	return infer.CreateResponse[ProjectTagState]{ID: id, Output: read.State}, nil
}

func (r ProjectTag) Read(ctx context.Context, req infer.ReadRequest[ProjectTagArgs, ProjectTagState]) (infer.ReadResponse[ProjectTagArgs, ProjectTagState], error) {
	projectID, tagID, err := parseProjectTagID(req.ID)
	if err != nil {
		return infer.ReadResponse[ProjectTagArgs, ProjectTagState]{}, err
	}
	resp, err := r.client(ctx).ProjectOneWithResponse(ctx, &generated.ProjectOneParams{ProjectId: projectID})
	if err != nil {
		if client.IsNotFound(err) {
			return infer.ReadResponse[ProjectTagArgs, ProjectTagState]{ID: ""}, nil
		}
		return infer.ReadResponse[ProjectTagArgs, ProjectTagState]{}, err
	}
	if resp.JSON200 == nil || resp.JSON200.ProjectId == nil || *resp.JSON200.ProjectId == "" {
		return infer.ReadResponse[ProjectTagArgs, ProjectTagState]{}, fmt.Errorf("project.one returned incomplete project")
	}
	if resp.JSON200.Tags == nil {
		return infer.ReadResponse[ProjectTagArgs, ProjectTagState]{ID: ""}, nil
	}
	for _, tag := range *resp.JSON200.Tags {
		if tag.TagId == tagID {
			args := ProjectTagArgs{ProjectID: projectID, TagID: tagID}
			return infer.ReadResponse[ProjectTagArgs, ProjectTagState]{ID: req.ID, Inputs: args, State: ProjectTagState{ProjectTagArgs: args}}, nil
		}
	}
	return infer.ReadResponse[ProjectTagArgs, ProjectTagState]{ID: ""}, nil
}

func (r ProjectTag) Delete(ctx context.Context, req infer.DeleteRequest[ProjectTagState]) (infer.DeleteResponse, error) {
	projectID, tagID, err := parseProjectTagID(req.ID)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	_, err = r.client(ctx).TagRemoveFromProjectWithResponse(ctx, generated.TagRemoveFromProjectJSONRequestBody{ProjectId: projectID, TagId: tagID})
	if client.IsNotFound(err) {
		return infer.DeleteResponse{}, nil
	}
	return infer.DeleteResponse{}, err
}

func (r ProjectTag) WireDependencies(f infer.FieldSelector, args *ProjectTagArgs, state *ProjectTagState) {
}
