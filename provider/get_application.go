package dokploy

import (
	"context"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type GetApplicationArgs struct {
	ApplicationID string `pulumi:"applicationId"`
}

func (a *GetApplicationArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.ApplicationID, "The ID of the existing Dokploy application to read.")
}

type GetApplicationResult struct {
	ApplicationID   string  `pulumi:"applicationId"`
	Name            string  `pulumi:"name"`
	Description     *string `pulumi:"description,optional"`
	AppName         *string `pulumi:"appName,optional"`
	EnvironmentID   *string `pulumi:"environmentId,optional"`
	ServerID        *string `pulumi:"serverId,optional"`
	Status          *string `pulumi:"status,optional"`
	RegistryID      *string `pulumi:"registryId,optional"`
	BuildRegistryID *string `pulumi:"buildRegistryId,optional"`
}

func (o *GetApplicationResult) Annotate(n infer.Annotator) {
	n.Describe(&o.ApplicationID, "The stable Dokploy application ID.")
	n.Describe(&o.Name, "The application name.")
	n.Describe(&o.Description, "The optional description metadata.")
	n.Describe(&o.AppName, "The optional application name metadata.")
	n.Describe(&o.EnvironmentID, "The optional environment ID metadata.")
	n.Describe(&o.ServerID, "The optional server ID metadata.")
	n.Describe(&o.Status, "The optional application status.")
	n.Describe(&o.RegistryID, "The optional registry ID metadata.")
	n.Describe(&o.BuildRegistryID, "The optional build registry ID metadata.")
}

type GetApplication struct{ client clientFactory }

func (r *GetApplication) Annotate(n infer.Annotator) {
	n.SetToken("index", "getApplication")
	n.Describe(&r, "Read non-secret metadata for an existing Dokploy application by ID without managing it.")
}
func (r GetApplication) Invoke(ctx context.Context, req infer.FunctionRequest[GetApplicationArgs]) (infer.FunctionResponse[GetApplicationResult], error) {
	var empty infer.FunctionResponse[GetApplicationResult]
	id := req.Input.ApplicationID
	if err := validateLookupID("applicationId", id); err != nil {
		return empty, err
	}
	response, err := r.client(ctx).ApplicationOne(ctx, &generated.ApplicationOneParams{ApplicationId: id})
	obj, err := lookupDecode[lookupApplication]("application.one", response, err)
	if err != nil {
		return empty, err
	}
	if err := validateLookupIdentity("application.one", id, obj.ApplicationId, obj.Name); err != nil {
		return empty, err
	}
	return infer.FunctionResponse[GetApplicationResult]{Output: GetApplicationResult{ApplicationID: id, Name: *obj.Name, Description: obj.Description, AppName: obj.AppName, EnvironmentID: obj.EnvironmentId, ServerID: obj.ServerId, Status: obj.ApplicationStatus, RegistryID: obj.RegistryId, BuildRegistryID: obj.BuildRegistryId}}, nil
}
