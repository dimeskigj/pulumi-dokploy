package dokploy

import (
	"context"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// GetEnvironmentArgs identifies an existing Dokploy environment.
type GetEnvironmentArgs struct {
	EnvironmentID string `pulumi:"environmentId"`
}

func (a *GetEnvironmentArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.EnvironmentID, "The ID of the existing Dokploy environment to read.")
}

// GetEnvironmentResult contains only read-only, non-secret metadata.
type GetEnvironmentResult struct {
	EnvironmentID string  `pulumi:"environmentId"`
	Name          string  `pulumi:"name"`
	Description   *string `pulumi:"description,optional"`
	ProjectID     *string `pulumi:"projectId,optional"`
	IsDefault     *bool   `pulumi:"isDefault,optional"`
}

func (out *GetEnvironmentResult) Annotate(n infer.Annotator) {
	n.Describe(&out.EnvironmentID, "The stable Dokploy environment ID.")
	n.Describe(&out.Name, "The environment name.")
	n.Describe(&out.Description, "The optional description metadata.")
	n.Describe(&out.ProjectID, "The optional project ID metadata.")
	n.Describe(&out.IsDefault, "The optional is default metadata.")
}

type GetEnvironment struct{ client clientFactory }

func (r *GetEnvironment) Annotate(n infer.Annotator) {
	n.SetToken("index", "getEnvironment")
	n.Describe(&r, "Read non-secret metadata for an existing Dokploy environment by ID without managing it.")
}
func (r GetEnvironment) Invoke(ctx context.Context, req infer.FunctionRequest[GetEnvironmentArgs]) (infer.FunctionResponse[GetEnvironmentResult], error) {
	var empty infer.FunctionResponse[GetEnvironmentResult]
	id := req.Input.EnvironmentID
	if err := validateLookupID("environmentId", id); err != nil {
		return empty, err
	}
	response, err := r.client(ctx).EnvironmentOne(ctx, &generated.EnvironmentOneParams{EnvironmentId: id})
	obj, err := lookupDecode[lookupEnvironment]("environment.one", response, err)
	if err != nil {
		return empty, err
	}
	if err := validateLookupIdentity("environment.one", id, obj.EnvironmentId, obj.Name); err != nil {
		return empty, err
	}
	return infer.FunctionResponse[GetEnvironmentResult]{Output: GetEnvironmentResult{EnvironmentID: id, Name: *obj.Name, Description: obj.Description, ProjectID: obj.ProjectId, IsDefault: obj.IsDefault}}, nil
}
