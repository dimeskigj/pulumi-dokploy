package dokploy

import (
	"context"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// GetProjectArgs identifies an existing Dokploy project.
type GetProjectArgs struct {
	ProjectID string `pulumi:"projectId"`
}

func (a *GetProjectArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.ProjectID, "The ID of the existing Dokploy project to read.")
}

// GetProjectResult contains only read-only, non-secret metadata.
type GetProjectResult struct {
	ProjectID            string  `pulumi:"projectId"`
	Name                 string  `pulumi:"name"`
	Description          *string `pulumi:"description,optional"`
	DefaultEnvironmentID *string `pulumi:"defaultEnvironmentId,optional"`
}

func (out *GetProjectResult) Annotate(n infer.Annotator) {
	n.Describe(&out.ProjectID, "The stable Dokploy project ID.")
	n.Describe(&out.Name, "The project name.")
	n.Describe(&out.Description, "The optional description metadata.")
	n.Describe(&out.DefaultEnvironmentID, "The optional default environment ID metadata.")
}

type GetProject struct{ client clientFactory }

func (r *GetProject) Annotate(n infer.Annotator) {
	n.SetToken("index", "getProject")
	n.Describe(&r, "Read non-secret metadata for an existing Dokploy project by ID without managing it.")
}
func (r GetProject) Invoke(ctx context.Context, req infer.FunctionRequest[GetProjectArgs]) (infer.FunctionResponse[GetProjectResult], error) {
	var empty infer.FunctionResponse[GetProjectResult]
	id := req.Input.ProjectID
	if err := validateLookupID("projectId", id); err != nil {
		return empty, err
	}
	response, err := r.client(ctx).ProjectOne(ctx, &generated.ProjectOneParams{ProjectId: id})
	obj, err := lookupDecode[lookupProject]("project.one", response, err)
	if err != nil {
		return empty, err
	}
	if err := validateLookupIdentity("project.one", id, obj.ProjectId, obj.Name); err != nil {
		return empty, err
	}
	return infer.FunctionResponse[GetProjectResult]{Output: GetProjectResult{ProjectID: id, Name: *obj.Name, Description: obj.Description, DefaultEnvironmentID: obj.DefaultEnvironmentId}}, nil
}
