package dokploy

import (
	"context"

	"github.com/dimeskigj/pulumi-dokploy/internal/client/generated"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// GetRegistryArgs identifies an existing Dokploy registry.
type GetRegistryArgs struct {
	RegistryID string `pulumi:"registryId"`
}

func (a *GetRegistryArgs) Annotate(n infer.Annotator) {
	n.Describe(&a.RegistryID, "The ID of the existing Dokploy registry to read.")
}

// GetRegistryResult contains only read-only, non-secret metadata.
type GetRegistryResult struct {
	RegistryID   string  `pulumi:"registryId"`
	Name         string  `pulumi:"name"`
	URL          *string `pulumi:"url,optional"`
	Username     *string `pulumi:"username,optional"`
	ImagePrefix  *string `pulumi:"imagePrefix,optional"`
	ServerID     *string `pulumi:"serverId,optional"`
	RegistryType *string `pulumi:"registryType,optional"`
}

func (out *GetRegistryResult) Annotate(n infer.Annotator) {
	n.Describe(&out.RegistryID, "The stable Dokploy registry ID.")
	n.Describe(&out.Name, "The registry name.")
	n.Describe(&out.URL, "The optional URL metadata.")
	n.Describe(&out.Username, "The optional username metadata.")
	n.Describe(&out.ImagePrefix, "The optional image prefix metadata.")
	n.Describe(&out.ServerID, "The optional server ID metadata.")
	n.Describe(&out.RegistryType, "The optional registry type metadata.")
}

type GetRegistry struct{ client clientFactory }

func (r *GetRegistry) Annotate(n infer.Annotator) {
	n.SetToken("index", "getRegistry")
	n.Describe(&r, "Read non-secret metadata for an existing Dokploy registry by ID without managing it.")
}
func (r GetRegistry) Invoke(ctx context.Context, req infer.FunctionRequest[GetRegistryArgs]) (infer.FunctionResponse[GetRegistryResult], error) {
	var empty infer.FunctionResponse[GetRegistryResult]
	id := req.Input.RegistryID
	if err := validateLookupID("registryId", id); err != nil {
		return empty, err
	}
	response, err := r.client(ctx).RegistryOneWithResponse(ctx, &generated.RegistryOneParams{RegistryId: id})
	if err != nil {
		return empty, lookupError("registry.one", err)
	}
	if response == nil || response.JSON200 == nil {
		return empty, lookupResponseError("registry.one")
	}
	obj := response.JSON200
	if err := validateLookupIdentity("registry.one", id, &obj.RegistryId, obj.RegistryName); err != nil {
		return empty, err
	}
	return infer.FunctionResponse[GetRegistryResult]{Output: GetRegistryResult{RegistryID: id, Name: *obj.RegistryName, URL: safeRegistryURL(obj.RegistryUrl), Username: obj.Username, ImagePrefix: nullableValue(obj.ImagePrefix), ServerID: nullableValue(obj.ServerId), RegistryType: obj.RegistryType}}, nil
}
