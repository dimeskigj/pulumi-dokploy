package dokploy

import (
	"context"
	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/pulumi/pulumi-go-provider/infer"
)

type GetComposeArgs struct {
	ComposeID string `pulumi:"composeId"`
}

func (a *GetComposeArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.ComposeID, "The ID of the existing Dokploy Compose deployment to read.")
}

type GetComposeResult struct {
	ComposeID     string  `pulumi:"composeId"`
	Name          string  `pulumi:"name"`
	Description   *string `pulumi:"description,optional"`
	AppName       *string `pulumi:"appName,optional"`
	EnvironmentID *string `pulumi:"environmentId,optional"`
	ServerID      *string `pulumi:"serverId,optional"`
	Status        *string `pulumi:"status,optional"`
	ComposeType   *string `pulumi:"composeType,optional"`
}

func (o *GetComposeResult) Annotate(n infer.Annotator) {
	n.Describe(&o.ComposeID, "The stable Dokploy Compose ID.")
	n.Describe(&o.Name, "The Compose deployment name.")
	n.Describe(&o.Description, "The optional description metadata.")
	n.Describe(&o.AppName, "The optional application name metadata.")
	n.Describe(&o.EnvironmentID, "The optional environment ID metadata.")
	n.Describe(&o.ServerID, "The optional server ID metadata.")
	n.Describe(&o.Status, "The optional Compose status.")
	n.Describe(&o.ComposeType, "The optional Compose type.")
}

type GetCompose struct{ client clientFactory }

func (r *GetCompose) Annotate(n infer.Annotator) {
	n.SetToken("index", "getCompose")
	n.Describe(&r, "Read non-secret metadata for an existing Dokploy Compose deployment by ID without managing it.")
}
func (r GetCompose) Invoke(ctx context.Context, req infer.FunctionRequest[GetComposeArgs]) (infer.FunctionResponse[GetComposeResult], error) {
	var empty infer.FunctionResponse[GetComposeResult]
	id := req.Input.ComposeID
	if err := validateLookupID("composeId", id); err != nil {
		return empty, err
	}
	response, err := r.client(ctx).ComposeOne(ctx, &generated.ComposeOneParams{ComposeId: id})
	obj, err := lookupDecode[lookupCompose]("compose.one", response, err)
	if err != nil {
		return empty, err
	}
	if err := validateLookupIdentity("compose.one", id, obj.ComposeId, obj.Name); err != nil {
		return empty, err
	}
	return infer.FunctionResponse[GetComposeResult]{Output: GetComposeResult{ComposeID: id, Name: *obj.Name, Description: obj.Description, AppName: obj.AppName, EnvironmentID: obj.EnvironmentId, ServerID: obj.ServerId, Status: obj.ComposeStatus, ComposeType: obj.ComposeType}}, nil
}
